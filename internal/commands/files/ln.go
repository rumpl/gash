package files

import (
	"context"
	"fmt"
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
