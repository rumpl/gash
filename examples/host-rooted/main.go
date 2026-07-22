package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	gashfs "github.com/rumpl/gash/pkg/fs"
	"github.com/rumpl/gash/pkg/gash"
)

func main() {
	hostDir, err := os.MkdirTemp("", "gash-rooted-example-")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(hostDir)

	filesystem, err := gashfs.NewRooted(hostDir)
	if err != nil {
		panic(err)
	}
	shell, err := gash.New(gash.Options{
		FS:  filesystem,
		Cwd: "/",
	})
	if err != nil {
		panic(err)
	}
	result := shell.Exec(
		context.Background(),
		`mkdir -p output; printf 'written by gash\n' > output/result.txt; cat output/result.txt`,
		gash.ExecOptions{},
	)
	fmt.Print(result.Stdout)
	fmt.Fprint(os.Stderr, result.Stderr)
	if result.ExitCode != 0 {
		os.Exit(result.ExitCode)
	}

	hostData, err := os.ReadFile(filepath.Join(hostDir, "output", "result.txt"))
	if err != nil {
		panic(err)
	}
	fmt.Printf("host observed: %s", hostData)
}
