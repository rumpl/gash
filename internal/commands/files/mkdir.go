package files

import (
	"context"

	gfs "github.com/rumpl/gash/pkg/fs"
)

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
