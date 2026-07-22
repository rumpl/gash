package gash

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestNonInteractiveAliasesRequireExpandAliases(t *testing.T) {
	shell := newTestBash(t)
	result := shell.Exec(context.Background(), `alias hi='echo expanded'; hi`, ExecOptions{})
	if result.ExitCode != 127 || result.Stdout != "" || !strings.Contains(result.Stderr, "hi: command not found") {
		t.Fatalf("result=%+v", result)
	}
}

func TestShellStateDiscoveryAndTimeCommands(t *testing.T) {
	shell, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}
	result := shell.Exec(context.Background(), "shopt -s expand_aliases; alias hi='echo hello'\nhi\nunalias hi\nhi", ExecOptions{})
	if result.ExitCode != 127 || result.Stdout != "hello\n" || !strings.Contains(result.Stderr, "hi: command not found") {
		t.Fatalf("alias/unalias result = %#v", result)
	}

	result = shell.Exec(context.Background(), "history\nwhich ls sh definitely-not-host", ExecOptions{})
	if result.ExitCode != 1 {
		t.Fatalf("which exit=%d stderr=%q", result.ExitCode, result.Stderr)
	}
	if result.Stdout != "ls: gash built-in\nsh: gash built-in\n" {
		t.Fatalf("which stdout=%q", result.Stdout)
	}
	if strings.Contains(result.Stdout, "/bin/") || strings.Contains(result.Stdout, "/usr/bin/") {
		t.Fatalf("which consulted host-looking paths: %q", result.Stdout)
	}

	result = shell.Exec(context.Background(), "help date", ExecOptions{})
	if result.ExitCode != 0 || !strings.Contains(result.Stdout, "Usage: date [OPTION]... [+FORMAT]") {
		t.Fatalf("help date = %#v", result)
	}
}

func TestTimeoutRunsSafelyAndCancelsNestedUTF8Input(t *testing.T) {
	shell, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}
	result := shell.Exec(context.Background(), "printf 'héllo' | command timeout 1 cat", ExecOptions{})
	if result.ExitCode != 0 || result.Stdout != "héllo" || result.Stderr != "" {
		t.Fatalf("timeout utf8 = %#v", result)
	}

	result = shell.Exec(context.Background(), "command timeout 0.01 sh -c 'sleep 1'", ExecOptions{})
	if result.ExitCode != 124 {
		t.Fatalf("timeout nested exit=%d stdout=%q stderr=%q", result.ExitCode, result.Stdout, result.Stderr)
	}

	result = shell.Exec(context.Background(), "command timeout 1 '/bin/echo' quoted", ExecOptions{})
	if result.ExitCode != 0 || result.Stdout != "quoted\n" {
		t.Fatalf("timeout command quoting = %#v", result)
	}
}

func TestDateFormatsUTCAndDSTWithClockHook(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip(err)
	}
	shell, err := New(Options{Now: func() time.Time {
		return time.Date(2024, 3, 10, 1, 30, 0, 0, loc)
	}})
	if err != nil {
		t.Fatal(err)
	}
	result := shell.Exec(context.Background(), "date +%F_%T_%Z_%z\ndate -u +%F_%T_%Z_%z\ndate -d tomorrow +%F_%Z", ExecOptions{})
	want := "2024-03-10_01:30:00_EST_-0500\n2024-03-10_06:30:00_UTC_+0000\n2024-03-11_EDT\n"
	if result.ExitCode != 0 || result.Stdout != want || result.Stderr != "" {
		t.Fatalf("date result = %#v, want stdout %q", result, want)
	}
}

func TestTimeUsesSafeCommandPath(t *testing.T) {
	shell, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}
	result := shell.Exec(context.Background(), "command time echo ok", ExecOptions{})
	if result.ExitCode != 0 || result.Stdout != "ok\n" {
		t.Fatalf("time result = %#v", result)
	}
	if !strings.Contains(result.Stderr, "real\t") || strings.Contains(result.Stderr, "bash: echo: command not found") {
		t.Fatalf("unexpected time stderr=%q", result.Stderr)
	}
}
