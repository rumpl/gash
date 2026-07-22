package main

import (
	"context"
	"fmt"
	"os"

	"github.com/rumpl/gash/pkg/gash"
	"github.com/rumpl/gash/pkg/network"
)

func main() {
	policy := network.NewPolicy(network.Rule{
		Scheme:  "https",
		Host:    "example.com",
		Port:    "443",
		Path:    "/",
		Methods: []string{"GET"},
	})
	shell, err := gash.New(gash.Options{
		Network: &policy,
	})
	if err != nil {
		panic(err)
	}
	result := shell.Exec(
		context.Background(),
		`curl -s https://example.com/ | head -n 3`,
		gash.ExecOptions{},
	)
	fmt.Print(result.Stdout)
	fmt.Fprint(os.Stderr, result.Stderr)
	if result.ExitCode != 0 {
		os.Exit(result.ExitCode)
	}
}
