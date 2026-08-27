//go:build js && wasm

package gash

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestWasmStreamsSupportShellInputs(t *testing.T) {
	bash, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}
	result := bash.Exec(context.Background(), `
read first
printf 'stdin=%s\n' "$first"
printf 'beta\nalpha\n' | sort
cat <<EOF_HEREDOC
heredoc
EOF_HEREDOC
value=$(printf substitution)
printf 'command=%s\n' "$value"
`, ExecOptions{Stdin: "browser\n"})
	want := "stdin=browser\nalpha\nbeta\nheredoc\ncommand=substitution\n"
	if result.ExitCode != 0 || result.Stdout != want || result.Stderr != "" {
		t.Fatalf("result=%+v, want stdout %q", result, want)
	}
}

func TestWasmPipelineLimitAndCancellation(t *testing.T) {
	limited, err := New(Options{Limits: Limits{MaxOutputBytes: 64, MaxExecutionTime: time.Second}})
	if err != nil {
		t.Fatal(err)
	}
	result := limited.Exec(context.Background(), `yes wasm | cat`, ExecOptions{})
	if result.ExitCode != 126 || len(result.Stdout)+len(result.Stderr) > 64 {
		t.Fatalf("limited result=%+v", result)
	}

	cancelled, err := New(Options{Limits: Limits{MaxExecutionTime: 20 * time.Millisecond}})
	if err != nil {
		t.Fatal(err)
	}
	result = cancelled.Exec(context.Background(), `sleep 1 | cat`, ExecOptions{})
	if result.ExitCode != 124 || !strings.Contains(result.Stderr, "execution timed out") {
		t.Fatalf("cancelled result=%+v", result)
	}
}
