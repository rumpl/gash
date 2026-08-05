package commands

import (
	"context"
	"fmt"
	"strings"
)

const (
	virtualUserName  = "user"
	virtualUserID    = "1000"
	virtualGroupName = "user"
	virtualGroupID   = "1000"
)

type idSelection uint8

const (
	idDefault idSelection = iota
	idUser
	idGroup
	idGroups
)

// commandID reports gash's fixed virtual identity. It deliberately does not
// inspect the host user database, process credentials, or environment.
func commandID(_ context.Context, args []string, c *CommandContext) int {
	selection := idDefault
	name := false
	optionsDone := false

	for _, arg := range args {
		if optionsDone {
			return idUnsupportedOperand(c, arg)
		}
		switch arg {
		case "--":
			optionsDone = true
		case "--user":
			if !setIDSelection(&selection, idUser) {
				return idInvalidCombination(c)
			}
		case "--group":
			if !setIDSelection(&selection, idGroup) {
				return idInvalidCombination(c)
			}
		case "--groups":
			if !setIDSelection(&selection, idGroups) {
				return idInvalidCombination(c)
			}
		case "--name":
			name = true
		default:
			if !strings.HasPrefix(arg, "-") || arg == "-" {
				return idUnsupportedOperand(c, arg)
			}
			if strings.HasPrefix(arg, "--") {
				fmt.Fprintf(c.Stderr, "id: unrecognized option '%s'\n", arg)
				return 1
			}
			for _, option := range arg[1:] {
				switch option {
				case 'u':
					if !setIDSelection(&selection, idUser) {
						return idInvalidCombination(c)
					}
				case 'g':
					if !setIDSelection(&selection, idGroup) {
						return idInvalidCombination(c)
					}
				case 'G':
					if !setIDSelection(&selection, idGroups) {
						return idInvalidCombination(c)
					}
				case 'n':
					name = true
				default:
					fmt.Fprintf(c.Stderr, "id: invalid option -- '%c'\n", option)
					return 1
				}
			}
		}
	}

	if name && selection == idDefault {
		fmt.Fprintln(c.Stderr, "id: --name requires exactly one of --user, --group, or --groups")
		return 1
	}

	switch selection {
	case idUser:
		if name {
			fmt.Fprintln(c.Stdout, virtualUserName)
		} else {
			fmt.Fprintln(c.Stdout, virtualUserID)
		}
	case idGroup, idGroups:
		if name {
			fmt.Fprintln(c.Stdout, virtualGroupName)
		} else {
			fmt.Fprintln(c.Stdout, virtualGroupID)
		}
	default:
		fmt.Fprintf(c.Stdout, "uid=%s(%s) gid=%s(%s) groups=%s(%s)\n", virtualUserID, virtualUserName, virtualGroupID, virtualGroupName, virtualGroupID, virtualGroupName)
	}
	return 0
}

func setIDSelection(current *idSelection, next idSelection) bool {
	if *current != idDefault {
		return false
	}
	*current = next
	return true
}

func idInvalidCombination(c *CommandContext) int {
	fmt.Fprintln(c.Stderr, "id: cannot print more than one of user, group, or groups")
	return 1
}

func idUnsupportedOperand(c *CommandContext, operand string) int {
	fmt.Fprintf(c.Stderr, "id: user operands are unsupported: '%s'\n", operand)
	return 1
}
