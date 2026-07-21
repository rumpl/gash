package gash

import (
	"bufio"
	"context"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	gfs "github.com/rumpl/gash/fs"
)

func builtinCommands() []Command {
	return []Command{
		{"echo", commandEcho}, {"printf", commandPrintf}, {"pwd", commandPwd}, {"cat", commandCat}, {"ls", commandLS}, {"mkdir", commandMkdir}, {"touch", commandTouch}, {"rm", commandRM}, {"rmdir", commandRM}, {"cp", commandCP}, {"mv", commandMV}, {"ln", commandLN}, {"readlink", commandReadlink}, {"head", commandHead}, {"tail", commandTail}, {"wc", commandWC}, {"grep", commandGrep}, {"sort", commandSort}, {"uniq", commandUniq}, {"tee", commandTee}, {"basename", commandBasename}, {"dirname", commandDirname}, {"env", commandEnv}, {"printenv", commandPrintenv}, {"true", func(context.Context, []string, *CommandContext) int { return 0 }}, {"false", func(context.Context, []string, *CommandContext) int { return 1 }}, {"sleep", commandSleep}, {"seq", commandSeq}, {"base64", commandBase64}, {"md5sum", checksum("md5")}, {"sha1sum", checksum("sha1")}, {"sha256sum", checksum("sha256")}, {"hostname", simpleOutput("localhost")}, {"whoami", simpleOutput("user")}, {"clear", func(_ context.Context, _ []string, c *CommandContext) int {
			fmt.Fprint(c.Stdout, "\033[H\033[2J")
			return 0
		}},
	}
}
func simpleOutput(s string) CommandFunc {
	return func(_ context.Context, _ []string, c *CommandContext) int { fmt.Fprintln(c.Stdout, s); return 0 }
}
func abs(c *CommandContext, p string) string { return c.FS.Resolve(*c.Cwd, p) }
func report(c *CommandContext, name string, e error) int {
	fmt.Fprintf(c.Stderr, "%s: %v\n", name, e)
	return 1
}
func commandEcho(_ context.Context, args []string, c *CommandContext) int {
	newline := true
	if len(args) > 0 && args[0] == "-n" {
		newline = false
		args = args[1:]
	}
	fmt.Fprint(c.Stdout, strings.Join(args, " "))
	if newline {
		fmt.Fprintln(c.Stdout)
	}
	return 0
}
func commandPrintf(_ context.Context, args []string, c *CommandContext) int {
	if len(args) == 0 {
		return 0
	}
	format := strings.ReplaceAll(strings.ReplaceAll(args[0], "\\n", "\n"), "\\t", "\t")
	vals := make([]any, len(args)-1)
	for i, v := range args[1:] {
		vals[i] = v
	}
	fmt.Fprintf(c.Stdout, format, vals...)
	return 0
}
func commandPwd(_ context.Context, _ []string, c *CommandContext) int {
	fmt.Fprintln(c.Stdout, *c.Cwd)
	return 0
}
func commandCD(_ context.Context, args []string, c *CommandContext) int {
	dest := c.Env["HOME"]
	if len(args) > 0 {
		dest = args[0]
	}
	if dest == "-" {
		dest = c.Env["OLDPWD"]
	}
	p := abs(c, dest)
	st, e := c.FS.Stat(p)
	if e != nil {
		return report(c, "cd", e)
	}
	if st.Kind != gfs.Directory {
		return report(c, "cd", gfs.ErrNotDir)
	}
	c.Env["OLDPWD"] = *c.Cwd
	*c.Cwd = p
	c.Env["PWD"] = p
	return 0
}
func commandCat(_ context.Context, args []string, c *CommandContext) int {
	if len(args) == 0 {
		_, e := io.Copy(c.Stdout, c.Stdin)
		if e != nil {
			return report(c, "cat", e)
		}
		return 0
	}
	code := 0
	for _, name := range args {
		if name == "-" {
			io.Copy(c.Stdout, c.Stdin)
			continue
		}
		data, e := c.FS.ReadFile(abs(c, name))
		if e != nil {
			code = report(c, "cat: "+name, e)
			continue
		}
		c.Stdout.Write(data)
	}
	return code
}
func commandLS(_ context.Context, args []string, c *CommandContext) int {
	all, long := false, false
	var names []string
	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			all = all || strings.Contains(a, "a")
			long = long || strings.Contains(a, "l")
		} else {
			names = append(names, a)
		}
	}
	if len(names) == 0 {
		names = []string{"."}
	}
	code := 0
	for _, name := range names {
		entries, e := c.FS.ReadDir(abs(c, name))
		if e != nil {
			st, se := c.FS.Stat(abs(c, name))
			if se != nil {
				code = report(c, "ls: "+name, se)
				continue
			}
			entries = []gfs.Info{st}
		}
		for _, entry := range entries {
			base := path.Base(entry.Path)
			if !all && strings.HasPrefix(base, ".") {
				continue
			}
			if long {
				kind := "-"
				if entry.Kind == gfs.Directory {
					kind = "d"
				} else if entry.Kind == gfs.Symlink {
					kind = "l"
				}
				fmt.Fprintf(c.Stdout, "%srwxr-xr-x %8d %s\n", kind, entry.Size, base)
			} else {
				fmt.Fprintln(c.Stdout, base)
			}
		}
	}
	return code
}
func commandMkdir(_ context.Context, args []string, c *CommandContext) int {
	recursive := false
	code := 0
	for _, a := range args {
		if a == "-p" {
			recursive = true
			continue
		}
		if e := c.FS.Mkdir(abs(c, a), 0755, recursive); e != nil {
			code = report(c, "mkdir: "+a, e)
		}
	}
	return code
}
func commandTouch(_ context.Context, args []string, c *CommandContext) int {
	code := 0
	for _, a := range args {
		p := abs(c, a)
		if _, e := c.FS.Stat(p); e == nil {
			continue
		}
		if e := c.FS.WriteFile(p, nil, 0644); e != nil {
			code = report(c, "touch: "+a, e)
		}
	}
	return code
}
func commandRM(_ context.Context, args []string, c *CommandContext) int {
	recursive, force := false, false
	var names []string
	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			recursive = recursive || strings.Contains(a, "r") || strings.Contains(a, "R")
			force = force || strings.Contains(a, "f")
		} else {
			names = append(names, a)
		}
	}
	code := 0
	for _, a := range names {
		if e := c.FS.Remove(abs(c, a), recursive); e != nil && !force {
			code = report(c, "rm: "+a, e)
		}
	}
	return code
}
func commandCP(_ context.Context, args []string, c *CommandContext) int {
	if len(args) != 2 {
		return report(c, "cp", fmt.Errorf("expected source and destination"))
	}
	data, e := c.FS.ReadFile(abs(c, args[0]))
	if e != nil {
		return report(c, "cp", e)
	}
	if e = c.FS.WriteFile(abs(c, args[1]), data, 0644); e != nil {
		return report(c, "cp", e)
	}
	return 0
}
func commandMV(_ context.Context, args []string, c *CommandContext) int {
	if len(args) != 2 {
		return report(c, "mv", fmt.Errorf("expected source and destination"))
	}
	if e := c.FS.Rename(abs(c, args[0]), abs(c, args[1])); e != nil {
		return report(c, "mv", e)
	}
	return 0
}
func commandLN(_ context.Context, args []string, c *CommandContext) int {
	if len(args) == 3 && args[0] == "-s" {
		if e := c.FS.Symlink(args[1], abs(c, args[2])); e != nil {
			return report(c, "ln", e)
		}
		return 0
	}
	return report(c, "ln", fmt.Errorf("only symbolic links are supported"))
}
func commandReadlink(_ context.Context, args []string, c *CommandContext) int {
	if len(args) != 1 {
		return 1
	}
	v, e := c.FS.Readlink(abs(c, args[0]))
	if e != nil {
		return report(c, "readlink", e)
	}
	fmt.Fprintln(c.Stdout, v)
	return 0
}
func readInputs(args []string, c *CommandContext) ([]byte, error) {
	if len(args) == 0 {
		return io.ReadAll(c.Stdin)
	}
	var out []byte
	for _, a := range args {
		d, e := c.FS.ReadFile(abs(c, a))
		if e != nil {
			return nil, e
		}
		out = append(out, d...)
	}
	return out, nil
}
func commandHead(_ context.Context, args []string, c *CommandContext) int {
	n := 10
	if len(args) >= 2 && args[0] == "-n" {
		n, _ = strconv.Atoi(args[1])
		args = args[2:]
	}
	d, e := readInputs(args, c)
	if e != nil {
		return report(c, "head", e)
	}
	lines := strings.SplitAfter(string(d), "\n")
	if n < len(lines) {
		lines = lines[:n]
	}
	fmt.Fprint(c.Stdout, strings.Join(lines, ""))
	return 0
}
func commandTail(_ context.Context, args []string, c *CommandContext) int {
	n := 10
	if len(args) >= 2 && args[0] == "-n" {
		n, _ = strconv.Atoi(args[1])
		args = args[2:]
	}
	d, e := readInputs(args, c)
	if e != nil {
		return report(c, "tail", e)
	}
	lines := strings.SplitAfter(string(d), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if n < len(lines) {
		lines = lines[len(lines)-n:]
	}
	fmt.Fprint(c.Stdout, strings.Join(lines, ""))
	return 0
}
func commandWC(_ context.Context, args []string, c *CommandContext) int {
	mode := "l"
	if len(args) > 0 && strings.HasPrefix(args[0], "-") {
		mode = strings.TrimPrefix(args[0], "-")
		args = args[1:]
	}
	d, e := readInputs(args, c)
	if e != nil {
		return report(c, "wc", e)
	}
	var n int
	switch mode {
	case "c":
		n = len(d)
	case "w":
		n = len(strings.Fields(string(d)))
	default:
		n = bytesCount(d, '\n')
	}
	fmt.Fprintf(c.Stdout, "%d\n", n)
	return 0
}
func bytesCount(d []byte, b byte) int {
	n := 0
	for _, v := range d {
		if v == b {
			n++
		}
	}
	return n
}
func commandGrep(_ context.Context, args []string, c *CommandContext) int {
	ignore, invert, number := false, false, false
	var rest []string
	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			ignore = ignore || strings.Contains(a, "i")
			invert = invert || strings.Contains(a, "v")
			number = number || strings.Contains(a, "n")
		} else {
			rest = append(rest, a)
		}
	}
	if len(rest) == 0 {
		return 2
	}
	pattern := rest[0]
	if ignore {
		pattern = "(?i)" + pattern
	}
	re, e := regexp.Compile(pattern)
	if e != nil {
		return report(c, "grep", e)
	}
	d, e := readInputs(rest[1:], c)
	if e != nil {
		return report(c, "grep", e)
	}
	found := false
	scan := bufio.NewScanner(strings.NewReader(string(d)))
	line := 0
	for scan.Scan() {
		line++
		match := re.MatchString(scan.Text())
		if invert {
			match = !match
		}
		if match {
			found = true
			if number {
				fmt.Fprintf(c.Stdout, "%d:", line)
			}
			fmt.Fprintln(c.Stdout, scan.Text())
		}
	}
	if found {
		return 0
	}
	return 1
}
func commandSort(_ context.Context, args []string, c *CommandContext) int {
	d, e := readInputs(args, c)
	if e != nil {
		return report(c, "sort", e)
	}
	lines := strings.Split(strings.TrimSuffix(string(d), "\n"), "\n")
	sort.Strings(lines)
	for _, l := range lines {
		fmt.Fprintln(c.Stdout, l)
	}
	return 0
}
func commandUniq(_ context.Context, args []string, c *CommandContext) int {
	d, e := readInputs(args, c)
	if e != nil {
		return report(c, "uniq", e)
	}
	last := "\x00"
	for _, l := range strings.Split(strings.TrimSuffix(string(d), "\n"), "\n") {
		if l != last {
			fmt.Fprintln(c.Stdout, l)
			last = l
		}
	}
	return 0
}
func commandTee(_ context.Context, args []string, c *CommandContext) int {
	d, e := io.ReadAll(c.Stdin)
	if e != nil {
		return 1
	}
	c.Stdout.Write(d)
	code := 0
	for _, a := range args {
		if e := c.FS.WriteFile(abs(c, a), d, 0644); e != nil {
			code = report(c, "tee", e)
		}
	}
	return code
}
func commandBasename(_ context.Context, args []string, c *CommandContext) int {
	if len(args) > 0 {
		fmt.Fprintln(c.Stdout, path.Base(args[0]))
	}
	return 0
}
func commandDirname(_ context.Context, args []string, c *CommandContext) int {
	if len(args) > 0 {
		fmt.Fprintln(c.Stdout, path.Dir(args[0]))
	}
	return 0
}
func commandEnv(_ context.Context, _ []string, c *CommandContext) int {
	keys := make([]string, 0, len(c.Env))
	for k := range c.Env {
		if k != "?" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(c.Stdout, "%s=%s\n", k, c.Env[k])
	}
	return 0
}
func commandPrintenv(_ context.Context, args []string, c *CommandContext) int {
	if len(args) == 0 {
		return commandEnv(context.Background(), nil, c)
	}
	code := 0
	for _, a := range args {
		v, ok := c.Env[a]
		if !ok {
			code = 1
		} else {
			fmt.Fprintln(c.Stdout, v)
		}
	}
	return code
}
func commandSleep(ctx context.Context, args []string, c *CommandContext) int {
	if len(args) == 0 {
		return 1
	}
	seconds, e := strconv.ParseFloat(args[0], 64)
	if e != nil {
		return report(c, "sleep", e)
	}
	select {
	case <-time.After(time.Duration(seconds * float64(time.Second))):
		return 0
	case <-ctx.Done():
		return 124
	}
}
func commandSeq(_ context.Context, args []string, c *CommandContext) int {
	if len(args) == 0 {
		return 1
	}
	start, end := 1, 0
	if len(args) == 1 {
		end, _ = strconv.Atoi(args[0])
	} else {
		start, _ = strconv.Atoi(args[0])
		end, _ = strconv.Atoi(args[1])
	}
	for i := start; i <= end; i++ {
		fmt.Fprintln(c.Stdout, i)
	}
	return 0
}
func commandBase64(_ context.Context, args []string, c *CommandContext) int {
	decode := len(args) > 0 && (args[0] == "-d" || args[0] == "--decode")
	if decode {
		args = args[1:]
	}
	d, e := readInputs(args, c)
	if e != nil {
		return report(c, "base64", e)
	}
	if decode {
		out, e := base64.StdEncoding.DecodeString(strings.TrimSpace(string(d)))
		if e != nil {
			return report(c, "base64", e)
		}
		c.Stdout.Write(out)
	} else {
		fmt.Fprintln(c.Stdout, base64.StdEncoding.EncodeToString(d))
	}
	return 0
}
func checksum(kind string) CommandFunc {
	return func(_ context.Context, args []string, c *CommandContext) int {
		d, e := readInputs(args, c)
		if e != nil {
			return report(c, kind+"sum", e)
		}
		var sum string
		switch kind {
		case "md5":
			sum = fmt.Sprintf("%x", md5.Sum(d))
		case "sha1":
			sum = fmt.Sprintf("%x", sha1.Sum(d))
		default:
			sum = fmt.Sprintf("%x", sha256.Sum256(d))
		}
		name := "-"
		if len(args) > 0 {
			name = args[0]
		}
		fmt.Fprintf(c.Stdout, "%s  %s\n", sum, name)
		return 0
	}
}
