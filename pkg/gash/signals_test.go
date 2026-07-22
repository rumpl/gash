package gash

import (
	"context"
	"testing"
)

func TestVirtualSignalTrapAndKill(t *testing.T) {
	shell := newTestBash(t)
	result := shell.Exec(
		context.Background(),
		`trap 'echo got-int' INT; kill -INT $$; echo after`,
		ExecOptions{},
	)
	if result.ExitCode != 0 || result.Stdout != "got-int\nafter\n" || result.Stderr != "" {
		t.Fatalf("result=%+v", result)
	}
}

func TestVirtualTrapPrintsSelectedDefinitions(t *testing.T) {
	shell := newTestBash(t)
	result := shell.Exec(context.Background(), `trap 'echo interrupted' INT; trap -p INT`, ExecOptions{})
	if result.ExitCode != 0 || result.Stdout != "trap -- \"echo interrupted\" INT\n" || result.Stderr != "" {
		t.Fatalf("result=%+v", result)
	}
}

func TestVirtualTrapPrintsExitDefinition(t *testing.T) {
	shell := newTestBash(t)
	result := shell.Exec(context.Background(), `trap 'echo trapped' EXIT; trap -p EXIT`, ExecOptions{})
	if result.ExitCode != 0 || result.Stdout != "trap -- \"echo trapped\" EXIT\ntrapped\n" || result.Stderr != "" {
		t.Fatalf("result=%+v", result)
	}
}

func TestExitTrapStillUsesInterpreterBehavior(t *testing.T) {
	shell := newTestBash(t)
	result := shell.Exec(context.Background(), `trap 'echo exiting' EXIT; echo body`, ExecOptions{})
	if result.ExitCode != 0 || result.Stdout != "body\nexiting\n" || result.Stderr != "" {
		t.Fatalf("result=%+v", result)
	}
}

func TestVirtualKillRejectsHostPID(t *testing.T) {
	shell := newTestBash(t)
	result := shell.Exec(context.Background(), `kill -INT 1`, ExecOptions{})
	if result.ExitCode != 1 || result.Stderr != "kill: (1) - No such process\n" {
		t.Fatalf("result=%+v", result)
	}
}
