package commands

import (
	"context"
	"fmt"
	"strings"

	"github.com/rumpl/gash/internal/commandhelp"
)

const (
	unameSysname  = "Gash"
	unameNodename = "localhost"
	unameRelease  = "1.0.0"
	unameVersion  = "#1 gash"
	unameMachine  = "virtual"
)

func commandUname(_ context.Context, args []string, c *CommandContext) int {
	selected := [5]bool{}
	optionsSeen := false
	endOptions := false

	for _, arg := range args {
		if endOptions {
			return unameUnexpectedOperand(c, arg)
		}
		if arg == "--" {
			endOptions = true
			continue
		}

		switch arg {
		case "--all":
			selected = [5]bool{true, true, true, true, true}
			optionsSeen = true
		case "--kernel-name":
			selected[0] = true
			optionsSeen = true
		case "--nodename":
			selected[1] = true
			optionsSeen = true
		case "--kernel-release":
			selected[2] = true
			optionsSeen = true
		case "--kernel-version":
			selected[3] = true
			optionsSeen = true
		case "--machine":
			selected[4] = true
			optionsSeen = true
		default:
			if strings.HasPrefix(arg, "--") {
				return commandhelp.UnknownOption(c, "uname", arg)
			}
			if !strings.HasPrefix(arg, "-") || arg == "-" {
				return unameUnexpectedOperand(c, arg)
			}
			if len(arg) == 1 {
				return unameUnexpectedOperand(c, arg)
			}
			for _, option := range arg[1:] {
				switch option {
				case 'a':
					selected = [5]bool{true, true, true, true, true}
				case 's':
					selected[0] = true
				case 'n':
					selected[1] = true
				case 'r':
					selected[2] = true
				case 'v':
					selected[3] = true
				case 'm':
					selected[4] = true
				default:
					return commandhelp.UnknownOption(c, "uname", "-"+string(option))
				}
				optionsSeen = true
			}
		}
	}

	if !optionsSeen {
		selected[0] = true
	}
	values := [...]string{unameSysname, unameNodename, unameRelease, unameVersion, unameMachine}
	fields := make([]string, 0, len(values))
	for index, value := range values {
		if selected[index] {
			fields = append(fields, value)
		}
	}
	fmt.Fprintln(c.Stdout, strings.Join(fields, " "))
	return 0
}

func unameUnexpectedOperand(c *CommandContext, operand string) int {
	fmt.Fprintf(c.Stderr, "uname: extra operand '%s'\n", operand)
	return 1
}
