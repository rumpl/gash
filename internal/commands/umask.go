package commands

import (
	"context"
	"fmt"
	iofs "io/fs"
	"strconv"
	"strings"
)

func commandUmask(_ context.Context, args []string, c *CommandContext) int {
	if c.Umask == nil {
		mask := iofs.FileMode(0o022)
		c.Umask = &mask
	}

	symbolic, reusable := false, false
	for len(args) > 0 && strings.HasPrefix(args[0], "-") {
		switch args[0] {
		case "-S":
			symbolic = true
		case "-p":
			reusable = true
		case "--":
			args = args[1:]
			goto parsedOptions
		default:
			fmt.Fprintf(c.Stderr, "umask: %s: invalid option\n", args[0])
			return 1
		}
		args = args[1:]
	}

parsedOptions:
	if len(args) > 1 {
		fmt.Fprintln(c.Stderr, "umask: too many arguments")
		return 1
	}
	if len(args) == 1 {
		mask, err := parseUmask(args[0], *c.Umask)
		if err != nil {
			fmt.Fprintf(c.Stderr, "umask: %s: invalid symbolic mode operator\n", args[0])
			return 1
		}
		*c.Umask = mask
	}

	if len(args) == 0 || symbolic || reusable {
		switch {
		case symbolic:
			fmt.Fprintln(c.Stdout, formatSymbolicUmask(*c.Umask))
		case reusable:
			fmt.Fprintf(c.Stdout, "umask %04o\n", *c.Umask&0o777)
		default:
			fmt.Fprintf(c.Stdout, "%04o\n", *c.Umask&0o777)
		}
	}
	return 0
}

func parseUmask(spec string, current iofs.FileMode) (iofs.FileMode, error) {
	if n, err := strconv.ParseUint(spec, 8, 16); err == nil && n <= 0o777 {
		return iofs.FileMode(n), nil
	}

	allowed := (^current) & 0o777
	for _, clause := range strings.Split(spec, ",") {
		i := strings.IndexAny(clause, "+-=")
		if i < 0 {
			return 0, fmt.Errorf("missing operator")
		}
		who, op, permissions := clause[:i], clause[i], clause[i+1:]
		if who == "" || strings.Contains(who, "a") {
			who = "ugo"
		}
		if strings.ContainsAny(permissions, "ugoXst") {
			return 0, fmt.Errorf("unsupported permission")
		}
		bits := iofs.FileMode(0)
		for _, permission := range permissions {
			switch permission {
			case 'r':
				bits |= 4
			case 'w':
				bits |= 2
			case 'x':
				bits |= 1
			default:
				return 0, fmt.Errorf("invalid permission")
			}
		}
		for _, class := range who {
			shift := 0
			switch class {
			case 'u':
				shift = 6
			case 'g':
				shift = 3
			case 'o':
			default:
				return 0, fmt.Errorf("invalid class")
			}
			classMask := iofs.FileMode(7 << shift)
			classBits := bits << shift
			switch op {
			case '+':
				allowed |= classBits
			case '-':
				allowed &^= classBits
			case '=':
				allowed = (allowed &^ classMask) | classBits
			}
		}
	}
	return (^allowed) & 0o777, nil
}

func formatSymbolicUmask(mask iofs.FileMode) string {
	allowed := (^mask) & 0o777
	return fmt.Sprintf("u=%s,g=%s,o=%s", permissionText((allowed>>6)&7), permissionText((allowed>>3)&7), permissionText(allowed&7))
}

func permissionText(bits iofs.FileMode) string {
	var out strings.Builder
	if bits&4 != 0 {
		out.WriteByte('r')
	}
	if bits&2 != 0 {
		out.WriteByte('w')
	}
	if bits&1 != 0 {
		out.WriteByte('x')
	}
	return out.String()
}
