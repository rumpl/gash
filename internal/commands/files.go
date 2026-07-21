package commands

import (
	"context"
	"fmt"
	iofs "io/fs"
	"strings"

	gfs "github.com/rumpl/gash/pkg/fs"
)

func commandLS(_ context.Context, args []string, c *CommandContext) int {
	all, long := false, false
	var names []string
	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			all = all || strings.Contains(a, "a")
			long = long || strings.Contains(a, "l")
		} else {
			names = append(names, a)
		}
	}
	if len(names) == 0 {
		names = []string{"."}
	}
	code := 0
	for _, name := range names {
		entries, e := gfs.ReadDir(c.FS, abs(c, name))
		if e != nil {
			st, se := gfs.Stat(c.FS, abs(c, name))
			if se != nil {
				code = report(c, "ls: "+name, se)
				continue
			}
			entries = []iofs.DirEntry{fileInfoEntry{st}}
		}
		for _, entry := range entries {
			base := entry.Name()
			if !all && strings.HasPrefix(base, ".") {
				continue
			}
			if long {
				info, _ := entry.Info()
				kind := "-"
				if entry.IsDir() {
					kind = "d"
				} else if entry.Type()&iofs.ModeSymlink != 0 {
					kind = "l"
				}
				fmt.Fprintf(c.Stdout, "%srwxr-xr-x %8d %s\n", kind, info.Size(), base)
			} else {
				fmt.Fprintln(c.Stdout, base)
			}
		}
	}
	return code
}

func commandMkdir(_ context.Context, args []string, c *CommandContext) int {
	recursive := false
	code := 0
	for _, a := range args {
		if a == "-p" {
			recursive = true
			continue
		}
		var e error
		if recursive {
			e = gfs.MkdirAll(c.FS, abs(c, a), 0o755)
		} else {
			e = gfs.Mkdir(c.FS, abs(c, a), 0o755)
		}
		if e != nil {
			code = report(c, "mkdir: "+a, e)
		}
	}
	return code
}

func commandTouch(_ context.Context, args []string, c *CommandContext) int {
	code := 0
	for _, a := range args {
		p := abs(c, a)
		if _, e := gfs.Stat(c.FS, p); e == nil {
			continue
		}
		if e := gfs.WriteFile(c.FS, p, nil, 0o644); e != nil {
			code = report(c, "touch: "+a, e)
		}
	}
	return code
}

func commandRM(_ context.Context, args []string, c *CommandContext) int {
	recursive, force := false, false
	var names []string
	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			recursive = recursive || strings.Contains(a, "r") || strings.Contains(a, "R")
			force = force || strings.Contains(a, "f")
		} else {
			names = append(names, a)
		}
	}
	code := 0
	for _, a := range names {
		var e error
		if recursive {
			e = gfs.RemoveAll(c.FS, abs(c, a))
		} else {
			e = gfs.Remove(c.FS, abs(c, a))
		}
		if e != nil && !force {
			code = report(c, "rm: "+a, e)
		}
	}
	return code
}

func commandCP(_ context.Context, args []string, c *CommandContext) int {
	if len(args) != 2 {
		return report(c, "cp", fmt.Errorf("expected source and destination"))
	}
	data, e := gfs.ReadFile(c.FS, abs(c, args[0]))
	if e != nil {
		return report(c, "cp", e)
	}
	if e = gfs.WriteFile(c.FS, abs(c, args[1]), data, 0o644); e != nil {
		return report(c, "cp", e)
	}
	return 0
}

func commandMV(_ context.Context, args []string, c *CommandContext) int {
	if len(args) != 2 {
		return report(c, "mv", fmt.Errorf("expected source and destination"))
	}
	if e := gfs.Rename(c.FS, abs(c, args[0]), abs(c, args[1])); e != nil {
		return report(c, "mv", e)
	}
	return 0
}

func commandLN(_ context.Context, args []string, c *CommandContext) int {
	if len(args) == 3 && args[0] == "-s" {
		if e := gfs.Symlink(c.FS, args[1], abs(c, args[2])); e != nil {
			return report(c, "ln", e)
		}
		return 0
	}
	return report(c, "ln", fmt.Errorf("only symbolic links are supported"))
}

func commandReadlink(_ context.Context, args []string, c *CommandContext) int {
	if len(args) != 1 {
		return 1
	}
	v, e := gfs.Readlink(c.FS, abs(c, args[0]))
	if e != nil {
		return report(c, "readlink", e)
	}
	fmt.Fprintln(c.Stdout, v)
	return 0
}
