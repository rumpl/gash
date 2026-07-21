// Package command defines the dependency-neutral command execution contract.
package command

import (
	"context"
	"io"
	iofs "io/fs"
)

type Context struct {
	FS             iofs.FS
	Cwd            *string
	Env            map[string]string
	Stdin          io.Reader
	Stdout, Stderr io.Writer
}

type Func func(context.Context, []string, *Context) int

type Command struct {
	Name string
	Run  Func
}
