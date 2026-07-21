package gash_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/rumpl/gash/pkg/gash"
)

func TestPublicPackageCustomCommand(t *testing.T) {
	shell, err := gash.New(gash.Options{Commands: []gash.Command{{Name: "hello", Run: func(_ context.Context, args []string, c *gash.CommandContext) int {
		fmt.Fprintln(c.Stdout, "hello")
		return 0
	}}}})
	if err != nil {
		t.Fatal(err)
	}
	result := shell.Exec(context.Background(), "hello", gash.ExecOptions{})
	if result.ExitCode != 0 || result.Stdout != "hello\n" {
		t.Fatalf("%+v", result)
	}
}
