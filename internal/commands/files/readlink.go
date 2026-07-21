package files

import (
	"context"
	"fmt"

	gfs "github.com/rumpl/gash/pkg/fs"
)

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
