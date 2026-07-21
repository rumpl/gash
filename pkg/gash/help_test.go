package gash

import (
	"context"
	"strings"
	"testing"
)

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
