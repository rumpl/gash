package gash

import (
	"context"
	"fmt"

	"github.com/rumpl/gash/internal/commandutil"
	gfs "github.com/rumpl/gash/pkg/fs"
	"mvdan.cc/sh/v3/interp"
)

func (b *Bash) reportCDDiagnostic(ctx context.Context, argv []string) {
	if len(argv) > 2 {
		return
	}
	handler := interp.HandlerCtx(ctx)
	requested := ""
	if len(argv) == 1 {
		requested = handler.Env.Get("HOME").String()
	} else {
		requested = argv[1]
	}
	if requested == "-" {
		requested = handler.Env.Get("OLDPWD").String()
	}
	info, err := gfs.Stat(b.FS, resolve(handler.Dir, requested))
	if err != nil {
		fmt.Fprintf(handler.Stderr, "bash: cd: %s: %s\n", requested, commandutil.ErrorText(err))
		return
	}
	if !info.IsDir() {
		fmt.Fprintf(handler.Stderr, "bash: cd: %s: Not a directory\n", requested)
	}
}
