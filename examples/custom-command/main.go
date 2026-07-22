package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/rumpl/gash/pkg/gash"
)

func main() {
	upper := gash.Command{
		Name: "upper",
		Run: func(_ context.Context, args []string, commandCtx *gash.CommandContext) int {
			input, err := io.ReadAll(commandCtx.Stdin)
			if err != nil {
				fmt.Fprintln(commandCtx.Stderr, "upper:", err)
				return 1
			}
			text := string(input)
			if len(args) > 0 {
				text = strings.Join(args, " ")
			}
			fmt.Fprint(commandCtx.Stdout, strings.ToUpper(text))
			return 0
		},
	}

	shell, err := gash.New(gash.Options{
		Commands: []gash.Command{upper},
	})
	if err != nil {
		panic(err)
	}
	result := shell.Exec(
		context.Background(),
		`printf 'custom commands compose with pipelines\n' | upper`,
		gash.ExecOptions{},
	)
	fmt.Print(result.Stdout)
	fmt.Fprint(os.Stderr, result.Stderr)
	if result.ExitCode != 0 {
		os.Exit(result.ExitCode)
	}
}
