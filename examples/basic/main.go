package main

import (
	"context"
	"fmt"
	"os"

	"github.com/rumpl/gash/pkg/gash"
)

func main() {
	shell, err := gash.New(gash.Options{})
	if err != nil {
		panic(err)
	}

	result := shell.Exec(
		context.Background(),
		`printf '%s\n' alpha beta gamma | grep 'a$' | sort -r`,
		gash.ExecOptions{},
	)
	fmt.Print(result.Stdout)
	fmt.Fprint(os.Stderr, result.Stderr)
	if result.ExitCode != 0 {
		os.Exit(result.ExitCode)
	}
}
