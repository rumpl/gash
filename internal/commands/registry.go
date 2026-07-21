package commands

import (
	"context"
	"fmt"
)

func Builtins() []Command {
	return []Command{
		{Name: "echo", Run: commandEcho},
		{Name: "printf", Run: commandPrintf},
		{Name: "pwd", Run: commandPwd},
		{Name: "cat", Run: commandCat},
		{Name: "ls", Run: commandLS},
		{Name: "mkdir", Run: commandMkdir},
		{Name: "touch", Run: commandTouch},
		{Name: "rm", Run: commandRM},
		{Name: "rmdir", Run: commandRmdir},
		{Name: "cp", Run: commandCPParity},
		{Name: "mv", Run: commandMV},
		{Name: "ln", Run: commandLNParity},
		{Name: "chmod", Run: commandChmod},
		{Name: "stat", Run: commandStat},
		{Name: "file", Run: commandFile},
		{Name: "tree", Run: commandTree},
		{Name: "du", Run: commandDu},
		{Name: "split", Run: commandSplit},
		{Name: "readlink", Run: commandReadlink},
		{Name: "head", Run: commandHead},
		{Name: "tail", Run: commandTail},
		{Name: "wc", Run: commandWC},
		{Name: "grep", Run: commandGrep},
		{Name: "egrep", Run: commandGrep},
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
		{Name: "join", Run: commandJoin},
		{Name: "nl", Run: commandNL},
		{Name: "fold", Run: commandFold},
		{Name: "expand", Run: commandExpand},
		{Name: "unexpand", Run: commandUnexpand},
		{Name: "column", Run: commandColumn},
		{Name: "od", Run: commandOD},
		{Name: "basename", Run: commandBasename},
		{Name: "dirname", Run: commandDirname},
		{Name: "env", Run: commandEnv},
		{Name: "printenv", Run: commandPrintenv},
		{Name: "true", Run: func(context.Context, []string, *CommandContext) int { return 0 }},
		{Name: "false", Run: func(context.Context, []string, *CommandContext) int { return 1 }},
		{Name: "sleep", Run: commandSleep},
		{Name: "seq", Run: commandSeq},
		{Name: "base64", Run: commandBase64},
		{Name: "md5sum", Run: checksum("md5")},
		{Name: "sha1sum", Run: checksum("sha1")},
		{Name: "sha256sum", Run: checksum("sha256")},
		{Name: "hostname", Run: simpleOutput("localhost")},
		{Name: "whoami", Run: simpleOutput("user")},
		{Name: "clear", Run: func(_ context.Context, _ []string, c *CommandContext) int {
			fmt.Fprint(c.Stdout, "\033[H\033[2J")
			return 0
		}},
	}
}
