package gash

import (
	"context"
	"testing"
)

func TestFactorPipelineReadsStdin(t *testing.T) {
	shell, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}
	result := shell.Exec(context.Background(), `echo '15 21' | factor`, ExecOptions{})
	if result.ExitCode != 0 || result.Stdout != "15: 3 5\n21: 3 7\n" || result.Stderr != "" {
		t.Fatalf("result=%+v", result)
	}
}
