package files

import (
	"context"
	"strings"

	gfs "github.com/rumpl/gash/pkg/fs"
)

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
