package commands

import (
	"context"
	"fmt"
	"io"
	"strings"

	gfs "github.com/rumpl/gash/pkg/fs"
)

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
	st, e := gfs.Stat(c.FS, p)
	if e != nil {
		return report(c, "cd", e)
	}
	if !st.IsDir() {
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
		data, e := gfs.ReadFile(c.FS, abs(c, name))
		if e != nil {
			code = report(c, "cat: "+name, e)
			continue
		}
		c.Stdout.Write(data)
	}
	return code
}
