package gash

import (
	"context"
	"strings"
	"testing"
	"testing/fstest"
)

func TestFailedRedirectionDoesNotAbortCommandList(t *testing.T) {
	shell, err := New(Options{FS: fstest.MapFS{".": {Mode: 0o555 | 0x80000000}}})
	if err != nil {
		t.Fatal(err)
	}
	result := shell.Exec(context.Background(), `echo before; echo x > /forbidden; echo after; exit 0`, ExecOptions{})
	if result.ExitCode != 0 || result.Stdout != "before\nafter\n" || !strings.Contains(result.Stderr, "filesystem is read-only") {
		t.Fatalf("result=%+v", result)
	}
}

func TestCDFailureReportsDiagnosticAndContinues(t *testing.T) {
	shell := newTestBash(t)
	result := shell.Exec(context.Background(), `cd /does-not-exist; echo status=$?`, ExecOptions{})
	if result.ExitCode != 0 || result.Stdout != "status=1\n" || result.Stderr != "bash: cd: /does-not-exist: No such file or directory\n" {
		t.Fatalf("result=%+v", result)
	}
}

func TestPipelineRightHandSideUsesSubshell(t *testing.T) {
	shell := newTestBash(t)
	result := shell.Exec(context.Background(), `v=outer; echo x | while read line; do v=inner; done; echo v=$v`, ExecOptions{})
	if result.ExitCode != 0 || result.Stdout != "v=outer\n" || result.Stderr != "" {
		t.Fatalf("result=%+v", result)
	}
}

func TestWaitWithVirtualJobArgumentDoesNotPanic(t *testing.T) {
	shell := newTestBash(t)
	result := shell.Exec(context.Background(), `sleep 0.01 & job=$!; wait "$job"; echo done`, ExecOptions{})
	if result.ExitCode != 0 || result.Stdout != "done\n" || result.Stderr != "" {
		t.Fatalf("result=%+v", result)
	}

	result = shell.Exec(context.Background(), `sleep 0.01 & command wait 123; echo command-done`, ExecOptions{})
	if result.ExitCode != 0 || result.Stdout != "command-done\n" || result.Stderr != "" {
		t.Fatalf("result=%+v", result)
	}
}

func TestExplicitSubshellRunsItsExitTrap(t *testing.T) {
	shell := newTestBash(t)
	result := shell.Exec(context.Background(), `trap 'echo trapped' EXIT; (trap 'echo subtrap' EXIT; echo subbody)`, ExecOptions{})
	if result.ExitCode != 0 || result.Stdout != "subbody\nsubtrap\ntrapped\n" || result.Stderr != "" {
		t.Fatalf("result=%+v", result)
	}
}

func TestDevNullIsAvailableToVirtualCommands(t *testing.T) {
	shell := newTestBash(t)
	result := shell.Exec(context.Background(), `awk 'BEGIN { print "hello" }' /dev/null`, ExecOptions{})
	if result.ExitCode != 0 || result.Stdout != "hello\n" || result.Stderr != "" {
		t.Fatalf("result=%+v", result)
	}
}

func TestFindExecUsesVirtualCommandDispatcher(t *testing.T) {
	shell := newTestBash(t)
	result := shell.Exec(context.Background(), `mkdir -p project; touch project/a project/b; find project -type f -exec printf '<%s>\n' {} +`, ExecOptions{})
	if result.ExitCode != 0 || result.Stdout != "<project/a>\n<project/b>\n" || result.Stderr != "" {
		t.Fatalf("result=%+v", result)
	}
}

func TestPasteAndPrintfQ(t *testing.T) {
	shell := newTestBash(t)
	result := shell.Exec(context.Background(), `printf 'a\nb\n' > a; printf '1\n2\n' > b; paste a b; printf '%q\n' 'hello world'; format='%q\n'; printf "$format" 'dynamic value'`, ExecOptions{})
	if result.ExitCode != 0 || result.Stdout != "a\t1\nb\t2\nhello\\ world\ndynamic\\ value\n" || result.Stderr != "" {
		t.Fatalf("result=%+v", result)
	}
}
