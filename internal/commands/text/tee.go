package text

import (
	"context"
	"io"

	gfs "github.com/rumpl/gash/pkg/fs"
)

func commandTee(_ context.Context, args []string, c *CommandContext) int {
	d, e := io.ReadAll(c.Stdin)
	if e != nil {
		return 1
	}
	c.Stdout.Write(d)
	code := 0
	for _, a := range args {
		if e := gfs.WriteFile(c.FS, abs(c, a), d, 0o644); e != nil {
			code = report(c, "tee", e)
		}
	}
	return code
}
