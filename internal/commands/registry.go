package commands

import (
	"context"
	"fmt"

	"github.com/rumpl/gash/internal/commandhelp"
	archivecommands "github.com/rumpl/gash/internal/commands/archive"
	curlcommands "github.com/rumpl/gash/internal/commands/curl"
	dataCommands "github.com/rumpl/gash/internal/commands/data"
	filecommands "github.com/rumpl/gash/internal/commands/files"
	sqlitecommands "github.com/rumpl/gash/internal/commands/sqlite"
	textcommands "github.com/rumpl/gash/internal/commands/text"
	"github.com/rumpl/gash/pkg/network"
)

func Builtins() []Command {
	return BuiltinsWithNetwork(nil)
}

func BuiltinsWithNetwork(policy *network.Policy) []Command {
	commands := []Command{
		{Name: "echo", Run: commandEcho},
		{Name: "printf", Run: commandPrintf},
		{Name: "pwd", Run: commandPwd},
		{Name: "cat", Run: commandCat},
		{Name: "basename", Run: commandBasename},
		{Name: "dirname", Run: commandDirname},
		{Name: "env", Run: commandEnv},
		{Name: "printenv", Run: commandPrintenv},
		{Name: "true", Run: func(context.Context, []string, *CommandContext) int { return 0 }},
		{Name: "false", Run: func(context.Context, []string, *CommandContext) int { return 1 }},
		{Name: "sleep", Run: commandSleep},
		{Name: "seq", Run: commandSeq},
		{Name: "date", Run: commandDate},
		{Name: "help", Run: commandHelp},
		{Name: "history", Run: commandHistory},
		{Name: "time", Run: commandTime},
		{Name: "timeout", Run: commandTimeout},
		{Name: "which", Run: commandWhich},
		{Name: "expr", Run: commandExpr},
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
	commands = append(commands, filecommands.Commands()...)
	commands = append(commands, textcommands.Commands()...)
	commands = append(commands, sqlitecommands.Commands()...)
	commands = append(commands, dataCommands.Commands()...)
	commands = append(commands, archivecommands.Commands()...)
	if policy != nil {
		commands = append(commands, curlcommands.Commands(*policy)...)
	}
	for index := range commands {
		info, ok := commandhelp.Lookup(commands[index].Name)
		if !ok {
			continue
		}
		run := commands[index].Run
		commands[index].Run = func(ctx context.Context, args []string, commandCtx *CommandContext) int {
			if commandhelp.Requested(args) {
				return commandhelp.Show(commandCtx, info)
			}
			return run(ctx, args, commandCtx)
		}
	}
	return commands
}
