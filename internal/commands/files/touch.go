package files

import (
	"context"
	"errors"
	iofs "io/fs"
	"time"

	gfs "github.com/rumpl/gash/pkg/fs"
)

func commandTouch(_ context.Context, args []string, c *CommandContext) int {
	now := time.Now()
	if c.Now != nil {
		now = c.Now()
	}
	code := 0
	for _, name := range args {
		full := abs(c, name)
		if _, err := gfs.Stat(c.FS, full); err == nil {
			if err := gfs.Chtimes(c.FS, full, now, now); err != nil {
				code = report(c, "touch: "+name, err)
			}
			continue
		} else if !errors.Is(err, iofs.ErrNotExist) {
			code = report(c, "touch: "+name, err)
			continue
		}
		if err := gfs.WriteFile(c.FS, full, nil, 0o644); err != nil {
			code = report(c, "touch: "+name, err)
		}
	}
	return code
}
