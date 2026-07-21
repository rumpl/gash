package files

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
