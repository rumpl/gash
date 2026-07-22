package gash

import (
	"context"
	"strings"
	"testing"
)

func TestDeclarationPrintOptions(t *testing.T) {
	shell := newTestBash(t)
	result := shell.Exec(context.Background(), `hidden=secret; export visible=public; export -p`, ExecOptions{})
	if result.ExitCode != 0 || !strings.Contains(result.Stdout, `declare -x visible="public"`) || strings.Contains(result.Stdout, "hidden=") || result.Stderr != "" {
		t.Fatalf("export -p result=%+v", result)
	}

	result = shell.Exec(context.Background(), `readonly fixed=value; readonly -p`, ExecOptions{})
	if result.ExitCode != 0 || !strings.Contains(result.Stdout, `declare -r fixed="value"`) || result.Stderr != "" {
		t.Fatalf("readonly -p result=%+v", result)
	}

	result = shell.Exec(context.Background(), `ordinary=value; declare -p`, ExecOptions{})
	if result.ExitCode != 0 || !strings.Contains(result.Stdout, `declare -- ordinary="value"`) || result.Stderr != "" {
		t.Fatalf("declare -p result=%+v", result)
	}
}
