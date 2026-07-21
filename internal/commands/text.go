package commands

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"

	gfs "github.com/rumpl/gash/pkg/fs"
)

func readInputs(args []string, c *CommandContext) ([]byte, error) {
	if len(args) == 0 {
		return io.ReadAll(c.Stdin)
	}
	var out []byte
	for _, a := range args {
		d, e := gfs.ReadFile(c.FS, abs(c, a))
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
		if e := gfs.WriteFile(c.FS, abs(c, a), d, 0644); e != nil {
			code = report(c, "tee", e)
		}
	}
	return code
}
