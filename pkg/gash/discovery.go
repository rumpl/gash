package gash

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"

	"mvdan.cc/sh/v3/interp"
)

var discoverableShellBuiltins = map[string]bool{
	":": true, "[": true, "alias": true, "break": true,
	"builtin": true, "cd": true, "command": true, "continue": true,
	"dirs": true, "echo": true, "eval": true, "exec": true, "exit": true,
	"false": true, "getopts": true, "jobs": true, "mapfile": true,
	"popd": true, "printf": true, "pushd": true, "pwd": true,
	"read": true, "readarray": true, "return": true, "set": true,
	"shift": true, "shopt": true, "source": true, "test": true,
	"trap": true, "true": true, "type": true, "umask": true,
	"unalias": true, "unset": true, "wait": true,
}

func (b *Bash) virtualCommandDiscovery(ctx context.Context, argv []string) ([]string, bool) {
	if len(argv) < 2 || argv[0] != "command" || argv[1] != "-v" && argv[1] != "-V" {
		return argv, false
	}
	verbose := argv[1] == "-V"
	names := argv[2:]
	if len(names) > 0 && names[0] == "--" {
		names = names[1:]
	}
	handler := interp.HandlerCtx(ctx)
	lastFound := true
	for _, requested := range names {
		name := normalizeDiscoveredName(requested)
		kind, found := b.discoverCommand(name)
		if !found || strings.Contains(name, "/") {
			lastFound = false
			continue
		}
		if verbose {
			fmt.Fprintf(handler.Stdout, "%s is %s\n", name, kind)
		} else {
			fmt.Fprintln(handler.Stdout, name)
		}
		lastFound = true
	}
	if lastFound {
		return []string{"true"}, true
	}
	return []string{"false"}, true
}

func (b *Bash) virtualTypeDiscovery(ctx context.Context, argv []string) ([]string, bool) {
	if len(argv) < 2 || argv[0] != "type" {
		return argv, false
	}
	names := argv[1:]
	if names[0] == "-a" {
		names = names[1:]
	} else if strings.HasPrefix(names[0], "-") {
		return argv, false
	}
	handler := interp.HandlerCtx(ctx)
	lastFound := true
	for _, requested := range names {
		name := normalizeDiscoveredName(requested)
		kind, found := b.discoverCommand(name)
		if !found || strings.Contains(name, "/") {
			fmt.Fprintf(handler.Stderr, "type: %s: not found\n", requested)
			lastFound = false
			continue
		}
		fmt.Fprintf(handler.Stdout, "%s is %s\n", name, kind)
		lastFound = true
	}
	if lastFound {
		return []string{"true"}, true
	}
	return []string{"false"}, true
}

func (b *Bash) discoverCommand(name string) (string, bool) {
	if discoverableShellBuiltins[name] {
		return "a shell builtin", true
	}
	if name == "bash" || name == "sh" || name == "kill" {
		return "a gash command", true
	}
	if _, registered := b.commands[name]; registered {
		return "a gash command", true
	}
	return "", false
}

type positionalState struct {
	count atomic.Int64
}

func newPositionalState(count int) *positionalState {
	state := &positionalState{}
	state.count.Store(int64(count))
	return state
}

func virtualShiftFailure(argv []string, state *positionalState) ([]string, bool) {
	if len(argv) >= 2 && argv[0] == "set" && argv[1] == "--" {
		state.count.Store(int64(len(argv) - 2))
		return argv, false
	}
	if len(argv) == 0 || argv[0] != "shift" || len(argv) > 2 {
		return argv, false
	}
	amount := 1
	if len(argv) == 2 {
		parsed, err := strconv.Atoi(argv[1])
		if err != nil || parsed < 0 {
			return argv, false
		}
		amount = parsed
	}
	for {
		count := state.count.Load()
		if int64(amount) > count {
			return []string{"false"}, true
		}
		if state.count.CompareAndSwap(count, count-int64(amount)) {
			return argv, false
		}
	}
}

func normalizeDiscoveredName(name string) string {
	return strings.TrimPrefix(strings.TrimPrefix(name, "/bin/"), "/usr/bin/")
}
