package gash

import (
	"context"
	"strings"
	"testing"
)

func TestOnlyExportedVariablesReachCommandsAndChildShells(t *testing.T) {
	shell := newTestBash(t)
	result := shell.Exec(context.Background(), `
unset hidden visible
hidden=secret
export visible=public
printf 'child-hidden='; sh -c 'printf "<%s>\n" "$hidden"'
printf 'child-visible='; sh -c 'printf "<%s>\n" "$visible"'
env
`, ExecOptions{})
	if result.ExitCode != 0 || !strings.Contains(result.Stdout, "child-hidden=<>\nchild-visible=<public>\n") {
		t.Fatalf("result=%+v", result)
	}
	if strings.Contains(result.Stdout, "hidden=secret") || strings.Contains(result.Stdout, "IFS=") {
		t.Fatalf("unexported variables leaked into env: %q", result.Stdout)
	}
	if !strings.Contains(result.Stdout, "visible=public\n") {
		t.Fatalf("exported variable missing from env: %q", result.Stdout)
	}
}
