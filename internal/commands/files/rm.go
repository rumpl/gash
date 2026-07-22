package files

import (
	"context"
	"errors"
	iofs "io/fs"
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
	for _, name := range names {
		full := abs(c, name)
		if _, err := gfs.Lstat(c.FS, full); err != nil {
			if force && errors.Is(err, iofs.ErrNotExist) {
				continue
			}
			code = report(c, "rm: "+name, err)
			continue
		}
		var err error
		if recursive {
			err = gfs.RemoveAll(c.FS, full)
		} else {
			err = gfs.Remove(c.FS, full)
		}
		if err != nil {
			code = report(c, "rm: "+name, err)
		}
	}
	return code
}
