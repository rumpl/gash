package files

import (
	"context"
	"errors"
	"fmt"
	iofs "io/fs"
	"path"
	"strings"

	"github.com/rumpl/gash/internal/commandhelp"
	gfs "github.com/rumpl/gash/pkg/fs"
)

const realpathMaxSymlinks = 40

func commandRealpath(_ context.Context, args []string, c *CommandContext) int {
	missing := false
	operands := make([]string, 0, len(args))
	options := true
	for _, arg := range args {
		if options && arg == "--" {
			options = false
			continue
		}
		if options && strings.HasPrefix(arg, "-") && arg != "-" {
			switch arg {
			case "-m", "--canonicalize-missing":
				missing = true
			case "-e", "--canonicalize-existing":
				missing = false
			default:
				return commandhelp.UnknownOption(c, "realpath", arg)
			}
			continue
		}
		operands = append(operands, arg)
	}
	if len(operands) == 0 {
		fmt.Fprintln(c.Stderr, "realpath: missing operand")
		return 1
	}

	status := 0
	for _, operand := range operands {
		resolved, err := resolveRealpath(c, operand, missing)
		if err != nil {
			fmt.Fprintf(c.Stderr, "realpath: %s: %s\n", operand, realpathErrorText(err))
			status = 1
			continue
		}
		fmt.Fprintln(c.Stdout, resolved)
	}
	return status
}

func resolveRealpath(c *CommandContext, operand string, missing bool) (string, error) {
	// Resolve and clean the operand first so lexical a/../b forms do not
	// require the discarded component to exist.
	pending := strings.Split(abs(c, operand), "/")

	resolved := make([]string, 0, len(pending))
	links := 0
	for len(pending) > 0 {
		component := pending[0]
		pending = pending[1:]
		switch component {
		case "", ".":
			continue
		case "..":
			if len(resolved) > 0 {
				resolved = resolved[:len(resolved)-1]
			}
			continue
		}

		candidate := "/" + path.Join(append(resolved, component)...)
		info, err := gfs.Lstat(c.FS, candidate)
		if err != nil {
			if missing && errors.Is(err, iofs.ErrNotExist) {
				resolved = append(resolved, component)
				continue
			}
			return "", err
		}
		if info.Mode()&iofs.ModeSymlink == 0 {
			resolved = append(resolved, component)
			continue
		}

		links++
		if links > realpathMaxSymlinks {
			return "", gfs.ErrLoop
		}
		target, err := gfs.VirtualReadlink(c.FS, candidate)
		if err != nil {
			return "", err
		}
		targetParts := strings.Split(target, "/")
		if strings.HasPrefix(target, "/") {
			resolved = resolved[:0]
		}
		pending = append(targetParts, pending...)
	}
	if len(resolved) == 0 {
		return "/", nil
	}
	return "/" + path.Join(resolved...), nil
}

func realpathErrorText(err error) string {
	if errors.Is(err, gfs.ErrLoop) {
		return gfs.ErrLoop.Error()
	}
	if errors.Is(err, iofs.ErrNotExist) {
		return "No such file or directory"
	}
	if errors.Is(err, iofs.ErrPermission) {
		return "Permission denied"
	}
	var pathError *iofs.PathError
	if errors.As(err, &pathError) {
		return pathError.Err.Error()
	}
	return err.Error()
}
