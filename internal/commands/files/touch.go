package files

import (
	"context"

	gfs "github.com/rumpl/gash/pkg/fs"
)

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
