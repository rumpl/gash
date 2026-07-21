package files

import (
	iofs "io/fs"

	"github.com/rumpl/gash/internal/command"
	"github.com/rumpl/gash/internal/commandutil"
)

type CommandContext = command.Context

type fileInfoEntry struct {
	iofs.FileInfo
}

func (entry fileInfoEntry) Type() iofs.FileMode {
	return entry.Mode().Type()
}

func (entry fileInfoEntry) Info() (iofs.FileInfo, error) {
	return entry.FileInfo, nil
}

func abs(ctx *CommandContext, name string) string {
	return commandutil.Abs(ctx, name)
}

func report(ctx *CommandContext, name string, err error) int {
	return commandutil.Report(ctx, name, err)
}
