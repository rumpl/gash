package files

import (
	"context"
	"fmt"
	iofs "io/fs"
	"path"
	"strconv"
	"strings"

	gfs "github.com/rumpl/gash/pkg/fs"
)

func commandChmod(_ context.Context, args []string, c *CommandContext) int {
	recursive, verbose := false, false
	for len(args) > 0 && strings.HasPrefix(args[0], "-") {
		flag := args[0]
		if flag == "--" {
			args = args[1:]
			break
		}
		if strings.Contains(flag, "R") {
			recursive = true
		}
		if strings.Contains(flag, "v") {
			verbose = true
		}
		if !strings.ContainsAny(flag, "Rv") {
			break
		}
		args = args[1:]
	}
	if len(args) < 2 {
		return report(c, "chmod", fmt.Errorf("missing operand"))
	}
	spec, targets := args[0], args[1:]
	code := 0
	for _, target := range targets {
		if err := chmodPath(c, abs(c, target), spec, recursive, verbose, target); err != nil {
			code = report(c, "chmod: "+target, err)
		}
	}
	return code
}

func chmodPath(c *CommandContext, name, spec string, recursive, verbose bool, display string) error {
	info, err := gfs.Stat(c.FS, name)
	if err != nil {
		return err
	}
	mode, err := parseMode(spec, info.Mode())
	if err != nil {
		return err
	}
	if err = gfs.Chmod(c.FS, name, mode); err != nil {
		return err
	}
	if verbose {
		fmt.Fprintf(c.Stdout, "mode of '%s' changed to %04o\n", display, mode)
	}
	if recursive && info.IsDir() {
		entries, err := gfs.ReadDir(c.FS, name)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if err := chmodPath(c, path.Join(name, entry.Name()), spec, true, verbose, path.Join(display, entry.Name())); err != nil {
				return err
			}
		}
	}
	return nil
}

func parseMode(spec string, current iofs.FileMode) (iofs.FileMode, error) {
	if n, err := strconv.ParseUint(spec, 8, 32); err == nil {
		return iofs.FileMode(n), nil
	}
	mode := current.Perm()
	for _, part := range strings.Split(spec, ",") {
		i := strings.IndexAny(part, "+-=")
		if i < 0 {
			return 0, fmt.Errorf("invalid mode")
		}
		who, op, perms := part[:i], part[i], part[i+1:]
		if who == "" || strings.Contains(who, "a") {
			who = "ugo"
		}
		bits := iofs.FileMode(0)
		if strings.Contains(perms, "r") {
			bits |= 4
		}
		if strings.Contains(perms, "w") {
			bits |= 2
		}
		if strings.ContainsAny(perms, "xX") {
			bits |= 1
		}
		for _, w := range who {
			shift := 0
			if w == 'u' {
				shift = 6
			} else if w == 'g' {
				shift = 3
			} else if w != 'o' {
				return 0, fmt.Errorf("invalid mode")
			}
			mask := bits << shift
			switch op {
			case '+':
				mode |= mask
			case '-':
				mode &^= mask
			case '=':
				mode = (mode &^ (7 << shift)) | mask
			default:
				return 0, fmt.Errorf("invalid mode")
			}
		}
	}
	return mode, nil
}
