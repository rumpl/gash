package text

import "github.com/rumpl/gash/internal/command"

func Commands() []command.Command {
	return []command.Command{
		{Name: "head", Run: commandHead},
		{Name: "tail", Run: commandTail},
		{Name: "wc", Run: commandWC},
		{Name: "grep", Run: commandGrep},
		{Name: "egrep", Run: commandGrep},
		{Name: "fgrep", Run: commandFGrep},
		{Name: "sort", Run: commandSort},
		{Name: "uniq", Run: commandUniq},
		{Name: "tee", Run: commandTee},
		{Name: "rev", Run: commandRev},
		{Name: "tac", Run: commandTac},
		{Name: "tr", Run: commandTr},
		{Name: "cut", Run: commandCut},
		{Name: "strings", Run: commandStrings},
		{Name: "paste", Run: commandPaste},
		{Name: "comm", Run: commandComm},
		{Name: "diff", Run: commandDiff},
		{Name: "join", Run: commandJoin},
		{Name: "nl", Run: commandNL},
		{Name: "fold", Run: commandFold},
		{Name: "expand", Run: commandExpand},
		{Name: "unexpand", Run: commandUnexpand},
		{Name: "column", Run: commandColumn},
		{Name: "od", Run: commandOD},
		{Name: "awk", Run: commandAwk},
		{Name: "sed", Run: commandSed},
		{Name: "rg", Run: commandRg},
	}
}
