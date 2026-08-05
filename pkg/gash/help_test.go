package gash

import (
	"context"
	"strings"
	"testing"
)

func TestSha512sumDiscovery(t *testing.T) {
	shell := newTestBash(t)
	result := shell.Exec(context.Background(), `command -v sha512sum; which sha512sum; help sha512sum`, ExecOptions{})
	if result.ExitCode != 0 || !strings.Contains(result.Stdout, "sha512sum\nsha512sum: gash built-in\n") || !strings.Contains(result.Stdout, "Usage: sha512sum [OPTION]... [FILE]...") || result.Stderr != "" {
		t.Fatalf("result=%+v", result)
	}
}

func TestLsHelpThroughShell(t *testing.T) {
	shell := newTestBash(t)
	result := shell.Exec(context.Background(), "ls --help", ExecOptions{})
	if result.ExitCode != 0 {
		t.Fatalf("exit=%d stderr=%q", result.ExitCode, result.Stderr)
	}
	if !strings.Contains(result.Stdout, "Usage: ls [OPTION]... [FILE]...") {
		t.Fatalf("unexpected help output:\n%s", result.Stdout)
	}
}
