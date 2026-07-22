package sqlite

import "github.com/rumpl/gash/internal/command"

func Commands() []command.Command {
	return []command.Command{{Name: "sqlite3", Run: commandSQLite3}}
}
