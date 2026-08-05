package utilities

import "github.com/rumpl/gash/internal/command"

func Commands() []command.Command {
	return []command.Command{
		FactorCommand(),
		Command(),
		UUIDGenCommand(),
	}
}
