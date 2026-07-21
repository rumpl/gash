package commands

import (
	"context"
	"fmt"
	"path"
	"sort"
)

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
