package files

import "github.com/rumpl/gash/internal/command"

func Commands() []command.Command {
	return []command.Command{
		{Name: "ls", Run: commandLS},
		{Name: "mkdir", Run: commandMkdir},
		{Name: "mktemp", Run: commandMktemp},
		{Name: "touch", Run: commandTouch},
		{Name: "rm", Run: commandRM},
		{Name: "rmdir", Run: commandRmdir},
		{Name: "cp", Run: commandCPParity},
		{Name: "cmp", Run: commandCmp},
		{Name: "mv", Run: commandMV},
		{Name: "ln", Run: commandLNParity},
		{Name: "chmod", Run: commandChmod},
		{Name: "stat", Run: commandStat},
		{Name: "file", Run: commandFile},
		{Name: "tree", Run: commandTree},
		{Name: "du", Run: commandDu},
		{Name: "split", Run: commandSplit},
		{Name: "readlink", Run: commandReadlink},
		{Name: "find", Run: commandFind},
	}
}
