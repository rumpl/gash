//go:build !js || !wasm

package commands

import (
	"github.com/rumpl/gash/internal/command"
	sqlitecommands "github.com/rumpl/gash/internal/commands/sqlite"
)

func sqliteCommands() []command.Command {
	return sqlitecommands.Commands()
}
