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
	"false": true, "getopts": true, "hash": true, "jobs": true, "mapfile": true,
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

func (b *Bash) virtualHashDiscovery(ctx context.Context, argv []string) ([]string, bool) {
	if len(argv) == 0 || argv[0] != "hash" {
		return argv, false
	}
	handler := interp.HandlerCtx(ctx)
	if len(argv) == 1 {
		// Gash deliberately has no command hash table: discovery is always
		// performed against the current capability-scoped virtual registry.
		return []string{"true"}, true
	}
	if len(argv) == 2 && argv[1] == "-r" {
		return []string{"true"}, true
	}
	if len(argv) == 2 && argv[1] == "--help" {
		fmt.Fprint(handler.Stdout, "hash - validate virtual gash command names without caching host paths\n\nUsage: hash [-r] [NAME...]\n\nOptions:\n  -r       reset the empty virtual command hash table (no-op)\n  --help   display this help and exit\n\nNotes:\n  NAME resolution uses only shell built-ins and capability-scoped gash commands.\n  No host PATH entries are resolved, cached, or reported.\n")
		return []string{"true"}, true
	}
	failed := false
	for _, requested := range argv[1:] {
		if strings.HasPrefix(requested, "-") {
			fmt.Fprintf(handler.Stderr, "hash: unsupported option: %s\n", requested)
			failed = true
			continue
		}
		name := normalizeDiscoveredName(requested)
		if _, found := b.discoverCommand(name); !found || strings.Contains(requested, "/") {
			fmt.Fprintf(handler.Stderr, "hash: %s: not found\n", requested)
			failed = true
		}
	}
	if failed {
		return []string{"false"}, true
	}
	return []string{"true"}, true
}

func (b *Bash) virtualTypeDiscovery(_ context.Context, argv []string) ([]string, bool) {
	if len(argv) == 0 {
		return argv, false
	}
	args := argv
	if args[0] == "command" {
		args = args[1:]
		if len(args) > 0 && args[0] == "--" {
			args = args[1:]
		}
	}
	if len(args) > 0 && args[0] == "builtin" {
		args = args[1:]
		if len(args) > 0 && args[0] == "--" {
			args = args[1:]
		}
	}
	if len(args) == 0 || args[0] != "type" {
		return argv, false
	}
	// Direct type, builtin type, and command-dispatched variants would otherwise
	// reach mvdan's shell-native implementation and expose host PATH lookup.
	// Preserve the type operands while forcing the registered capability-scoped
	// implementation for every supported dispatch form.
	return append([]string{"/usr/bin/type"}, args[1:]...), true
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
