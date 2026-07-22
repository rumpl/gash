package gash

import (
	"context"
	"fmt"
	"strings"

	"mvdan.cc/sh/v3/interp"
)

var discoverableShellBuiltins = map[string]bool{
	":": true, "[": true, "alias": true, "bg": true, "break": true,
	"builtin": true, "cd": true, "command": true, "continue": true,
	"dirs": true, "echo": true, "eval": true, "exec": true, "exit": true,
	"false": true, "fg": true, "getopts": true, "mapfile": true,
	"popd": true, "printf": true, "pushd": true, "pwd": true,
	"read": true, "readarray": true, "return": true, "set": true,
	"shift": true, "shopt": true, "source": true, "test": true,
	"trap": true, "true": true, "type": true, "umask": true,
	"unalias": true, "unset": true, "wait": true,
}

func (b *Bash) virtualCommandDiscovery(ctx context.Context, argv []string) ([]string, bool) {
	if len(argv) < 2 || argv[0] != "command" || argv[1] != "-v" {
		return argv, false
	}
	names := argv[2:]
	if len(names) > 0 && names[0] == "--" {
		names = names[1:]
	}
	handler := interp.HandlerCtx(ctx)
	lastFound := true
	for _, requested := range names {
		name := normalizeDiscoveredName(requested)
		if strings.Contains(name, "/") {
			lastFound = false
			continue
		}
		_, registered := b.commands[name]
		switch {
		case discoverableShellBuiltins[name]:
			fmt.Fprintln(handler.Stdout, name)
			lastFound = true
		case name == "bash" || name == "sh" || name == "kill":
			fmt.Fprintln(handler.Stdout, "/usr/bin/"+name)
			lastFound = true
		case registered:
			fmt.Fprintln(handler.Stdout, "/usr/bin/"+name)
			lastFound = true
		default:
			lastFound = false
		}
	}
	if lastFound {
		return []string{"true"}, true
	}
	return []string{"false"}, true
}

func normalizeDiscoveredName(name string) string {
	return strings.TrimPrefix(strings.TrimPrefix(name, "/bin/"), "/usr/bin/")
}
