package gash

import (
	"context"
	"testing"
)

func TestShiftFailsBeyondPositionalArguments(t *testing.T) {
	shell := newTestBash(t)
	result := shell.Exec(context.Background(), `set --; shift; echo empty=$?; set -- one; shift; echo valid=$?; shift; echo exhausted=$?`, ExecOptions{})
	if result.ExitCode != 0 || result.Stdout != "empty=1\nvalid=0\nexhausted=1\n" || result.Stderr != "" {
		t.Fatalf("result=%+v", result)
	}
}
