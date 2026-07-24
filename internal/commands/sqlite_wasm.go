//go:build js && wasm

package commands

import "github.com/rumpl/gash/internal/command"

// modernc.org/sqlite does not support Go's js/wasm target.
func sqliteCommands() []command.Command {
	return nil
}
