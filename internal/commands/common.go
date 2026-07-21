package commands

import (
	"context"
	"fmt"
	iofs "io/fs"
	"path"
	"strings"

	"github.com/rumpl/gash/internal/command"
)

type (
	Command        = command.Command
	CommandFunc    = command.Func
	CommandContext = command.Context
)

type fileInfoEntry struct{ iofs.FileInfo }

func (e fileInfoEntry) Type() iofs.FileMode          { return e.Mode().Type() }
func (e fileInfoEntry) Info() (iofs.FileInfo, error) { return e.FileInfo, nil }

func simpleOutput(s string) CommandFunc {
	return func(_ context.Context, _ []string, c *CommandContext) int { fmt.Fprintln(c.Stdout, s); return 0 }
}

func resolve(base, name string) string {
	if strings.HasPrefix(name, "/") {
		return path.Clean(name)
	}
	return path.Clean(path.Join(base, name))
}
func abs(c *CommandContext, p string) string { return resolve(*c.Cwd, p) }
func report(c *CommandContext, name string, e error) int {
	fmt.Fprintf(c.Stderr, "%s: %v\n", name, e)
	return 1
}
