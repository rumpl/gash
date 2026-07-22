// Package command defines the dependency-neutral command execution contract.
package command

import (
	"context"
	"io"
	iofs "io/fs"
	"time"
)

type Context struct {
	FS  iofs.FS
	Cwd *string
	Env map[string]string
	// Umask is the current file creation mask. Commands that create filesystem
	// entries should apply CreationMode to their requested mode.
	Umask          *iofs.FileMode
	Stdin          io.Reader
	Stdout, Stderr io.Writer

	// Commands lists the registered in-process command names visible to the
	// current shell. It is informational for discovery built-ins such as help and
	// which; command execution must go through RunCommand.
	Commands []string
	// RunCommand executes argv through the embedding shell's safe command path.
	// It must not consult the host PATH.
	RunCommand func(context.Context, []string, *Context) int
	// Now returns the current time for date-like commands. A nil value means
	// time.Now.
	Now func() time.Time
}

func (c *Context) CreationMode(mode iofs.FileMode) iofs.FileMode {
	mask := iofs.FileMode(0o022)
	if c != nil && c.Umask != nil {
		mask = *c.Umask
	}
	return mode &^ mask
}

type Func func(context.Context, []string, *Context) int

type Command struct {
	Name string
	Run  Func
}
