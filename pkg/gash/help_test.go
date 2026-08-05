package gash

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestAdditiveCommandsHaveHelpAndDiscovery(t *testing.T) {
	// These commands extend gash beyond the pinned just-bash registry. Keep the
	// product inventory, help catalog, and both discovery paths in lockstep.
	for _, name := range []string{
		"cksum", "cmp", "factor", "id", "install", "mktemp", "realpath",
		"sha512sum", "shuf", "umask", "uname", "unlink", "uuidgen", "yes",
	} {
		t.Run(name, func(t *testing.T) {
			shell := newTestBash(t)
			script := fmt.Sprintf("command -v %s; which %s; help %s", name, name, name)
			result := shell.Exec(context.Background(), script, ExecOptions{})
			if result.ExitCode != 0 || result.Stderr != "" {
				t.Fatalf("result=%+v", result)
			}
			wantPrefix := fmt.Sprintf("%s\n%s: gash built-in\n%s - ", name, name, name)
			if !strings.HasPrefix(result.Stdout, wantPrefix) || !strings.Contains(result.Stdout, "\n\nUsage: ") {
				t.Fatalf("stdout=%q, want discovery prefix %q followed by catalog help", result.Stdout, wantPrefix)
			}
		})
	}
}

func TestSha512sumDiscovery(t *testing.T) {
	shell := newTestBash(t)
	result := shell.Exec(context.Background(), `command -v sha512sum; which sha512sum; help sha512sum`, ExecOptions{})
	if result.ExitCode != 0 || !strings.Contains(result.Stdout, "sha512sum\nsha512sum: gash built-in\n") || !strings.Contains(result.Stdout, "Usage: sha512sum [OPTION]... [FILE]...") || result.Stderr != "" {
		t.Fatalf("result=%+v", result)
	}
}

func TestSha512sumEndOfOptionsPreventsHelpInterception(t *testing.T) {
	shell, err := New(Options{Files: map[string]string{"/home/user/--help": ""}})
	if err != nil {
		t.Fatal(err)
	}
	result := shell.Exec(context.Background(), `sha512sum -- --help`, ExecOptions{})
	want := "cf83e1357eefb8bdf1542850d66d8007d620e4050b5715dc83f4a921d36ce9ce47d0d13c5d85f2b0ff8318d2877eec2f63b931bd47417a81a538327af927da3e  --help\n"
	if result.ExitCode != 0 || result.Stdout != want || result.Stderr != "" {
		t.Fatalf("result=%+v", result)
	}
}

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
