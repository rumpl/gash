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

func TestWorkingDirectoryIsCanonicalizedInsideVirtualRoot(t *testing.T) {
	shell, err := New(Options{Cwd: "/../../../../"})
	if err != nil {
		t.Fatal(err)
	}
	result := shell.Exec(context.Background(), `pwd; printf '%s\n' "$PWD"`, ExecOptions{})
	if result.ExitCode != 0 || result.Stdout != "/\n/\n" || result.Stderr != "" {
		t.Fatalf("configured cwd result=%+v", result)
	}
	result = shell.Exec(context.Background(), `pwd`, ExecOptions{Cwd: "/work/../../../"})
	if result.ExitCode != 0 || result.Stdout != "/\n" || result.Stderr != "" {
		t.Fatalf("execution cwd result=%+v", result)
	}
}

func TestInvalidWorkingDirectoriesAreRejectedBeforeExecution(t *testing.T) {
	for _, options := range []Options{
		{Cwd: "/missing"},
		{Cwd: "relative/path"},
		{Cwd: "/README.md", Files: map[string]string{"/README.md": "file"}},
	} {
		if shell, err := New(options); err == nil {
			t.Fatalf("New(%+v) unexpectedly succeeded: %+v", options, shell)
		}
	}

	shell := newTestBash(t)
	result := shell.Exec(context.Background(), `echo SHOULD_NOT_RUN`, ExecOptions{Cwd: "/missing"})
	if result.ExitCode != 1 || result.Stdout != "" || !strings.Contains(result.Stderr, "No such file or directory") {
		t.Fatalf("missing cwd result=%+v", result)
	}
	result = shell.Exec(context.Background(), `echo SHOULD_NOT_RUN`, ExecOptions{Cwd: "relative/path"})
	if result.ExitCode != 1 || result.Stdout != "" || !strings.Contains(result.Stderr, "must be absolute") {
		t.Fatalf("relative cwd result=%+v", result)
	}
	if result = shell.Exec(context.Background(), `printf file > /cwd-file`, ExecOptions{}); result.ExitCode != 0 {
		t.Fatalf("seed file result=%+v", result)
	}
	result = shell.Exec(context.Background(), `echo SHOULD_NOT_RUN`, ExecOptions{Cwd: "/cwd-file"})
	if result.ExitCode != 1 || result.Stdout != "" || !strings.Contains(result.Stderr, "not a directory") {
		t.Fatalf("file cwd result=%+v", result)
	}
}

func TestDefaultHomeIsVirtualRoot(t *testing.T) {
	memoryShell := newTestBash(t)
	result := memoryShell.Exec(context.Background(), `printf '%s\n' "$HOME"; cd ~; pwd`, ExecOptions{})
	if result.ExitCode != 0 || result.Stdout != "/\n/\n" || result.Stderr != "" {
		t.Fatalf("memory result=%+v", result)
	}

	filesystem := fstest.MapFS{".": {Mode: 0o555 | 0x80000000}}
	shell, err := New(Options{FS: filesystem})
	if err != nil {
		t.Fatal(err)
	}
	result = shell.Exec(context.Background(), `printf '%s:%s\n' "$HOME" "$PWD"; cd ~; pwd`, ExecOptions{})
	if result.ExitCode != 0 || result.Stdout != "/:/\n/\n" || result.Stderr != "" {
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

func TestBackgroundExecutionIsAsynchronousAndIsolated(t *testing.T) {
	shell := newTestBash(t)
	result := shell.Exec(context.Background(), `x=before; { sleep 0.05; x=background; echo child; } & p=$!; echo immediate-x=$x bang=$p; wait "$p"; echo wait=$? final-x=$x`, ExecOptions{})
	want := "immediate-x=before bang=2001\nchild\nwait=0 final-x=before\n"
	if result.ExitCode != 0 || result.Stdout != want || result.Stderr != "" {
		t.Fatalf("result=%+v", result)
	}
}

func TestWaitReportsUnknownVirtualJob(t *testing.T) {
	shell := newTestBash(t)
	result := shell.Exec(context.Background(), `wait 123`, ExecOptions{})
	if result.ExitCode != 127 || result.Stdout != "" || result.Stderr != "wait: 123: no such job\n" {
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
