package gash

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

const (
	internalTrapCommand = "__gash_virtual_trap"
	internalKillCommand = "__gash_virtual_kill"
)

var virtualSignals = map[string]int{
	"HUP":  1,
	"INT":  2,
	"QUIT": 3,
	"TERM": 15,
}

func rewriteVirtualSignalBuiltins(program syntax.Node) {
	syntax.Walk(program, func(node syntax.Node) bool {
		call, ok := node.(*syntax.CallExpr)
		if !ok || len(call.Args) == 0 {
			return true
		}
		name, ok := literalWord(call.Args[0])
		if !ok {
			return true
		}
		switch name {
		case "kill":
			setLiteralWord(call.Args[0], internalKillCommand)
		case "trap":
			if len(call.Args) > 1 {
				if option, literal := literalWord(call.Args[1]); literal && option == "-p" {
					setLiteralWord(call.Args[0], internalTrapCommand)
					break
				}
			}
			for _, argument := range call.Args[1:] {
				value, literal := literalWord(argument)
				if literal && isVirtualSignal(value) {
					setLiteralWord(call.Args[0], internalTrapCommand)
					break
				}
			}
		}
		return true
	})
}

func literalWord(word *syntax.Word) (string, bool) {
	if word == nil || len(word.Parts) != 1 {
		return "", false
	}
	literal, ok := word.Parts[0].(*syntax.Lit)
	if !ok {
		return "", false
	}
	return literal.Value, true
}

func setLiteralWord(word *syntax.Word, value string) {
	literal, ok := word.Parts[0].(*syntax.Lit)
	if ok {
		literal.Value = value
	}
}

func isVirtualSignal(value string) bool {
	_, _, ok := normalizeVirtualSignal(value)
	return ok
}

func normalizeVirtualSignal(value string) (string, int, bool) {
	value = strings.ToUpper(strings.TrimPrefix(value, "SIG"))
	if number, err := strconv.Atoi(value); err == nil {
		for name, signalNumber := range virtualSignals {
			if signalNumber == number {
				return name, number, true
			}
		}
		return "", number, false
	}
	number, ok := virtualSignals[value]
	return value, number, ok
}

func normalizeTrapSignal(value string) (string, bool) {
	value = strings.ToUpper(value)
	if value == "EXIT" || value == "0" {
		return "EXIT", true
	}
	signal, _, ok := normalizeVirtualSignal(value)
	return signal, ok
}

func observeExitTrap(argv []string, scope *executionScope) {
	if len(argv) < 2 || argv[0] != "trap" || argv[1] == "-p" {
		return
	}
	args := argv[1:]
	if args[0] == "--" {
		args = args[1:]
	}
	if len(args) == 0 {
		return
	}
	callback := "-"
	signals := args
	if len(args) > 1 {
		callback = args[0]
		signals = args[1:]
	}
	for _, value := range signals {
		if signal, ok := normalizeTrapSignal(value); ok && signal == "EXIT" {
			if callback == "-" {
				scope.setTrap("EXIT", "")
			} else {
				scope.setTrap("EXIT", callback)
			}
		}
	}
}

func (b *Bash) runVirtualTrap(_ context.Context, args []string, commandCtx *CommandContext, scope *executionScope) int {
	if len(args) == 0 || args[0] == "-p" {
		signals := []string{"HUP", "INT", "QUIT", "TERM", "EXIT"}
		if len(args) > 1 {
			signals = signals[:0]
			for _, value := range args[1:] {
				signal, ok := normalizeTrapSignal(value)
				if !ok {
					fmt.Fprintf(commandCtx.Stderr, "trap: %s: invalid signal specification\n", value)
					return 2
				}
				signals = append(signals, signal)
			}
		}
		for _, signal := range signals {
			if callback, ok := scope.trap(signal); ok {
				fmt.Fprintf(commandCtx.Stdout, "trap -- %q %s\n", callback, signal)
			}
		}
		return 0
	}
	callback := "-"
	signals := args
	if len(args) > 1 {
		callback = args[0]
		signals = args[1:]
	}
	for _, value := range signals {
		signal, _, ok := normalizeVirtualSignal(value)
		if !ok {
			fmt.Fprintf(commandCtx.Stderr, "trap: %s: invalid signal specification\n", value)
			return 2
		}
		if callback == "-" {
			scope.setTrap(signal, "")
		} else {
			scope.setTrap(signal, callback)
		}
	}
	return 0
}

func (b *Bash) runVirtualKill(ctx context.Context, args []string, commandCtx *CommandContext, depth int, scope *executionScope) int {
	if len(args) == 0 {
		fmt.Fprintln(commandCtx.Stderr, "kill: usage: kill [-s signal | -signal] pid ...")
		return 2
	}
	signal := "TERM"
	index := 0
	if args[index] == "-s" {
		if len(args) < 3 {
			fmt.Fprintln(commandCtx.Stderr, "kill: option requires an argument -- s")
			return 2
		}
		signal = args[index+1]
		index += 2
	} else if strings.HasPrefix(args[index], "-") && len(args[index]) > 1 {
		signal = strings.TrimPrefix(args[index], "-")
		index++
	}
	signal, number, ok := normalizeVirtualSignal(signal)
	if !ok {
		fmt.Fprintf(commandCtx.Stderr, "kill: %s: invalid signal specification\n", signal)
		return 2
	}
	if index >= len(args) {
		fmt.Fprintln(commandCtx.Stderr, "kill: usage: kill [-s signal | -signal] pid ...")
		return 2
	}
	signalShell := false
	for _, pid := range args[index:] {
		if pid == virtualPID {
			signalShell = true
			continue
		}
		if scope.signalJob(pid, 128+number) {
			continue
		}
		fmt.Fprintf(commandCtx.Stderr, "kill: (%s) - No such process\n", pid)
		return 1
	}
	if !signalShell {
		return 0
	}
	callback, trapped := scope.trap(signal)
	if !trapped {
		return 128 + number
	}
	code, _ := b.execute(
		ctx,
		callback,
		"",
		*commandCtx.Cwd,
		commandCtx.Env,
		nil,
		signal+" trap",
		commandCtx.Stdout,
		commandCtx.Stderr,
		depth+1,
		scope,
		true,
	)
	return code
}
