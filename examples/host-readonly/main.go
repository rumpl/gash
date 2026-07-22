package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/rumpl/gash/pkg/gash"
)

func main() {
	root := "."
	if len(os.Args) > 1 {
		root = os.Args[1]
	}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		panic(err)
	}

	// os.DirFS is convenient for trusted local use. It is read-only through
	// gash, but symlinks can resolve outside the selected directory.
	shell, err := gash.New(gash.Options{
		FS:  os.DirFS(absoluteRoot),
		Cwd: "/",
	})
	if err != nil {
		panic(err)
	}
	result := shell.Exec(
		context.Background(),
		`printf 'host files visible under /:\n'; ls`,
		gash.ExecOptions{},
	)
	fmt.Print(result.Stdout)
	fmt.Fprint(os.Stderr, result.Stderr)
	if result.ExitCode != 0 {
		os.Exit(result.ExitCode)
	}
}
