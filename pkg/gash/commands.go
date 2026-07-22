package gash

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

func (b *Bash) commandNames() []string {
	names := make([]string, 0, len(b.commands))
	for name := range b.commands {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (b *Bash) runCommandFromContext(ctx context.Context, argv []string, c *CommandContext, depth int, scope *executionScope) int {
	if len(argv) == 0 {
		return 0
	}
	if err := scope.chargeCommand(); err != nil {
		fmt.Fprintf(c.Stderr, "bash: %v\n", err)
		return 126
	}
	name := strings.TrimPrefix(strings.TrimPrefix(argv[0], "/bin/"), "/usr/bin/")
	if name == "bash" || name == "sh" {
		if depth >= b.limits.MaxCallDepth {
			fmt.Fprintln(c.Stderr, "bash: maximum call depth exceeded")
			return 126
		}
		args := argv[1:]
		if len(args) >= 2 && args[0] == "-c" {
			scriptName := ""
			params := []string(nil)
			if len(args) > 2 {
				scriptName = args[2]
				params = args[3:]
			}
			code, _ := b.execute(ctx, args[1], "", *c.Cwd, c.Env, params, scriptName, c.Stdout, c.Stderr, depth+1, scope, true)
			return code
		}
		fmt.Fprintf(c.Stderr, "%s: only -c execution is supported here\n", name)
		return 1
	}
	if name == "trap" || name == internalTrapCommand {
		return b.runVirtualTrap(ctx, argv[1:], c, scope)
	}
	if name == "kill" || name == internalKillCommand {
		return b.runVirtualKill(ctx, argv[1:], c, depth, scope)
	}
	cmd, ok := b.commands[name]
	if !ok {
		fmt.Fprintf(c.Stderr, "bash: %s: command not found\n", name)
		return 127
	}
	child := *c
	child.RunCommand = func(runCtx context.Context, childArgv []string, grandchild *CommandContext) int {
		return b.runCommandFromContext(runCtx, childArgv, grandchild, depth, scope)
	}
	return cmd.Run(ctx, argv[1:], &child)
}
