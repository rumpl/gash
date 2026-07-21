package files

import (
	"context"
	"fmt"
	iofs "io/fs"
	"strings"

	"github.com/rumpl/gash/internal/commandhelp"
	gfs "github.com/rumpl/gash/pkg/fs"
)

var lsHelp = commandhelp.Info{
	Name:    "ls",
	Summary: "list directory contents",
	Usage:   "ls [OPTION]... [FILE]...",
	Options: []string{
		"-a, --all            do not ignore entries starting with .",
		"-A, --almost-all     do not list . and ..",
		"-d, --directory      list directories themselves, not their contents",
		"-F, --classify       append indicator (one of */=>@) to entries",
		"-h, --human-readable with -l, print sizes like 1K 234M 2G etc.",
		"-l                   use a long listing format",
		"-r, --reverse        reverse order while sorting",
		"-R, --recursive      list subdirectories recursively",
		"-S                   sort by file size, largest first",
		"-t                   sort by time, newest first",
		"-1                   list one file per line",
		"    --help           display this help and exit",
	},
}

func commandLS(_ context.Context, args []string, c *CommandContext) int {
	if commandhelp.Requested(args) {
		return commandhelp.Show(c, lsHelp)
	}

	all, long := false, false
	var names []string
	for _, a := range args {
		if strings.HasPrefix(a, "--") {
			switch a {
			case "--all":
				all = true
			default:
				return commandhelp.UnknownOption(c, "ls", a)
			}
		} else if strings.HasPrefix(a, "-") {
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
