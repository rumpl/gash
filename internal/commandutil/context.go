// Package commandutil contains shared, dependency-neutral helpers for built-in commands.
package commandutil

import (
	"errors"
	"fmt"
	"io"
	iofs "io/fs"
	"path"
	"strings"

	"github.com/rumpl/gash/internal/command"
	gfs "github.com/rumpl/gash/pkg/fs"
)

func Resolve(base, name string) string {
	if strings.HasPrefix(name, "/") {
		return path.Clean(name)
	}
	return path.Clean(path.Join(base, name))
}

func Abs(ctx *command.Context, name string) string {
	return Resolve(*ctx.Cwd, name)
}

func Report(ctx *command.Context, name string, err error) int {
	fmt.Fprintf(ctx.Stderr, "%s: %s\n", name, ErrorText(err))
	return 1
}

func ErrorText(err error) string {
	switch {
	case errors.Is(err, iofs.ErrNotExist):
		return "No such file or directory"
	case errors.Is(err, iofs.ErrPermission):
		return "Permission denied"
	}
	var pathError *iofs.PathError
	if errors.As(err, &pathError) {
		return pathError.Err.Error()
	}
	return err.Error()
}

func ReadInputs(args []string, ctx *command.Context) ([]byte, error) {
	if len(args) == 0 {
		return io.ReadAll(ctx.Stdin)
	}
	var out []byte
	for _, name := range args {
		if name == "-" {
			data, err := io.ReadAll(ctx.Stdin)
			if err != nil {
				return nil, err
			}
			out = append(out, data...)
			continue
		}
		data, err := gfs.ReadFile(ctx.FS, Abs(ctx, name))
		if err != nil {
			return nil, err
		}
		out = append(out, data...)
	}
	return out, nil
}
