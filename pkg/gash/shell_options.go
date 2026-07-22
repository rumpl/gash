package gash

import (
	"context"
	"strings"
	"sync/atomic"
)

const forceClobberPrefix = "__gash_force_clobber__:"

type shellOptionContextKey struct{}

type shellOptionState struct {
	noclobber atomic.Bool
}

func withShellOptionState(ctx context.Context) (context.Context, *shellOptionState) {
	state := &shellOptionState{}
	return context.WithValue(ctx, shellOptionContextKey{}, state), state
}

func shellOptionsFromContext(ctx context.Context) *shellOptionState {
	state, _ := ctx.Value(shellOptionContextKey{}).(*shellOptionState)
	return state
}

func virtualSetOptions(argv []string, state *shellOptionState) ([]string, bool) {
	if len(argv) < 2 || argv[0] != "set" {
		return argv, false
	}
	if len(argv) == 3 && (argv[1] == "-o" || argv[1] == "+o") && argv[2] == "noclobber" {
		state.noclobber.Store(argv[1] == "-o")
		return []string{"true"}, true
	}
	option := argv[1]
	if len(option) < 2 || option[0] != '-' && option[0] != '+' || !strings.ContainsRune(option[1:], 'C') {
		return argv, false
	}
	state.noclobber.Store(option[0] == '-')
	remaining := strings.ReplaceAll(option[1:], "C", "")
	if remaining == "" {
		return []string{"true"}, true
	}
	rewritten := append([]string(nil), argv...)
	rewritten[1] = string(option[0]) + remaining
	return rewritten, true
}
