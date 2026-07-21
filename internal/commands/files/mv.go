package files

import (
	"context"
	"fmt"

	gfs "github.com/rumpl/gash/pkg/fs"
)

func commandMV(_ context.Context, args []string, c *CommandContext) int {
	if len(args) != 2 {
		return report(c, "mv", fmt.Errorf("expected source and destination"))
	}
	if e := gfs.Rename(c.FS, abs(c, args[0]), abs(c, args[1])); e != nil {
		return report(c, "mv", e)
	}
	return 0
}
