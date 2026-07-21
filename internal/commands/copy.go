package commands

import (
	"context"
	"fmt"
	iofs "io/fs"
	"path"
	"strings"

	gfs "github.com/rumpl/gash/pkg/fs"
)

func commandLNParity(_ context.Context, args []string, c *CommandContext) int {
	symbolic, force := false, false
	var paths []string
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			symbolic = symbolic || strings.Contains(arg, "s")
			force = force || strings.Contains(arg, "f")
		} else {
			paths = append(paths, arg)
		}
	}
	if len(paths) != 2 {
		return report(c, "ln", fmt.Errorf("missing file operand"))
	}
	dest := abs(c, paths[1])
	if force {
		_ = gfs.Remove(c.FS, dest)
	}
	var err error
	if symbolic {
		err = gfs.Symlink(c.FS, paths[0], dest)
	} else {
		err = gfs.Link(c.FS, abs(c, paths[0]), dest)
	}
	if err != nil {
		return report(c, "ln", err)
	}
	return 0
}

func commandCPParity(_ context.Context, args []string, c *CommandContext) int {
	recursive, noClobber, verbose, preserve := false, false, false, false
	var paths []string
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			recursive = recursive || strings.ContainsAny(arg, "rR")
			noClobber = noClobber || strings.Contains(arg, "n")
			verbose = verbose || strings.Contains(arg, "v")
			preserve = preserve || strings.Contains(arg, "p")
		} else {
			paths = append(paths, arg)
		}
	}
	if len(paths) < 2 {
		return report(c, "cp", fmt.Errorf("missing destination file operand"))
	}
	destArg := paths[len(paths)-1]
	sources := paths[:len(paths)-1]
	dest := abs(c, destArg)
	destInfo, destErr := gfs.Stat(c.FS, dest)
	destDir := destErr == nil && destInfo.IsDir()
	if len(sources) > 1 && !destDir {
		return report(c, "cp", fmt.Errorf("target '%s' is not a directory", destArg))
	}
	code := 0
	for _, srcArg := range sources {
		src := abs(c, srcArg)
		target := dest
		if destDir {
			target = path.Join(dest, path.Base(src))
		}
		if noClobber {
			if _, err := gfs.Stat(c.FS, target); err == nil {
				continue
			}
		}
		if src == target || strings.HasPrefix(target, src+"/") {
			fmt.Fprintf(c.Stderr, "cp: cannot copy '%s' into itself\n", srcArg)
			code = 1
			continue
		}
		if err := copyPath(c, src, target, recursive, preserve); err != nil {
			fmt.Fprintf(c.Stderr, "cp: cannot copy '%s': %v\n", srcArg, err)
			code = 1
		} else if verbose {
			fmt.Fprintf(c.Stdout, "'%s' -> '%s'\n", srcArg, target)
		}
	}
	return code
}

func copyPath(c *CommandContext, src, dst string, recursive, preserve bool) error {
	info, err := gfs.Lstat(c.FS, src)
	if err != nil {
		return err
	}
	if info.Mode()&iofs.ModeSymlink != 0 {
		if !recursive {
			return fmt.Errorf("symlink requires recursive copy")
		}
		target, err := gfs.Readlink(c.FS, src)
		if err != nil {
			return err
		}
		return gfs.Symlink(c.FS, target, dst)
	}
	if info.IsDir() {
		if !recursive {
			return fmt.Errorf("-r not specified; omitting directory")
		}
		if err := gfs.MkdirAll(c.FS, dst, info.Mode().Perm()); err != nil {
			return err
		}
		entries, err := gfs.ReadDir(c.FS, src)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if err := copyPath(c, path.Join(src, e.Name()), path.Join(dst, e.Name()), true, preserve); err != nil {
				return err
			}
		}
		return nil
	}
	data, err := gfs.ReadFile(c.FS, src)
	if err != nil {
		return err
	}
	mode := iofs.FileMode(0o644)
	if preserve {
		mode = info.Mode().Perm()
	}
	return gfs.WriteFile(c.FS, dst, data, mode)
}
