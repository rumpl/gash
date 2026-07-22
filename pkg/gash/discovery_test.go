package gash

import (
	"context"
	"testing"

	"github.com/rumpl/gash/pkg/network"
)

func TestCommandVUsesVirtualCommandRegistry(t *testing.T) {
	shell := newTestBash(t)
	result := shell.Exec(context.Background(), `
command -v curl >/dev/null; echo curl=$?
command -v realpath >/dev/null; echo realpath=$?
command -v ls
command -v echo
`, ExecOptions{})
	want := "curl=1\nrealpath=1\n/usr/bin/ls\necho\n"
	if result.ExitCode != 0 || result.Stdout != want || result.Stderr != "" {
		t.Fatalf("result=%+v", result)
	}
}

func TestCommandVReflectsOptionalCapabilities(t *testing.T) {
	policy := network.NewPolicy()
	shell, err := New(Options{Network: &policy})
	if err != nil {
		t.Fatal(err)
	}
	result := shell.Exec(context.Background(), `command -v curl`, ExecOptions{})
	if result.ExitCode != 0 || result.Stdout != "/usr/bin/curl\n" || result.Stderr != "" {
		t.Fatalf("result=%+v", result)
	}
}

func TestCommandVFindsCustomCommands(t *testing.T) {
	shell, err := New(Options{Commands: []Command{{Name: "custom", Run: func(_ context.Context, _ []string, c *CommandContext) int {
		return 0
	}}}})
	if err != nil {
		t.Fatal(err)
	}
	result := shell.Exec(context.Background(), `command -v custom; custom`, ExecOptions{})
	if result.ExitCode != 0 || result.Stdout != "/usr/bin/custom\n" || result.Stderr != "" {
		t.Fatalf("result=%+v", result)
	}
}
