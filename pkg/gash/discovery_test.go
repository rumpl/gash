package gash

import (
	"context"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/rumpl/gash/pkg/network"
)

func TestCommandVUsesVirtualCommandRegistry(t *testing.T) {
	shell := newTestBash(t)
	result := shell.Exec(context.Background(), `
command -v curl >/dev/null; echo curl=$?
command -v realpath >/dev/null; echo realpath=$?
command -v ls
command -v id
command -v echo
`, ExecOptions{})
	want := "curl=1\nrealpath=0\nls\nid\necho\n"
	if result.ExitCode != 0 || result.Stdout != want || result.Stderr != "" {
		t.Fatalf("result=%+v", result)
	}
}

func TestIDDiscoveryAndHelp(t *testing.T) {
	shell := newTestBash(t)
	result := shell.Exec(context.Background(), `command -v id; which id; help id`, ExecOptions{})
	if result.ExitCode != 0 || result.Stderr != "" {
		t.Fatalf("result=%+v", result)
	}
	wantPrefix := "id\nid: gash built-in\nid - print the fixed gash virtual user and group identity\n\nUsage: id [OPTION]...\n"
	if !strings.HasPrefix(result.Stdout, wantPrefix) {
		t.Fatalf("stdout=%q, want prefix %q", result.Stdout, wantPrefix)
	}
}

func TestUnlinkDiscoveryAndHelp(t *testing.T) {
	shell := newTestBash(t)
	result := shell.Exec(context.Background(), `command -v unlink; which unlink; help unlink; unlink --help`, ExecOptions{})
	if result.ExitCode != 0 || result.Stderr != "" {
		t.Fatalf("result=%+v", result)
	}
	wantPrefix := "unlink\nunlink: gash built-in\nunlink - remove one virtual filesystem file\n\nUsage: unlink FILE\n"
	if !strings.HasPrefix(result.Stdout, wantPrefix) {
		t.Fatalf("stdout=%q, want prefix %q", result.Stdout, wantPrefix)
	}
	if strings.Count(result.Stdout, "unlink - remove one virtual filesystem file") != 2 {
		t.Fatalf("help was not shown consistently: %q", result.Stdout)
	}
}

func TestVerboseCommandAndTypeDiscoveryAvoidFilesystemPaths(t *testing.T) {
	shell := newTestBash(t)
	result := shell.Exec(context.Background(), `command -V bash; type bash; type -a ls`, ExecOptions{})
	want := "bash is a gash command\nbash is a gash command\nls is a gash command\n"
	if result.ExitCode != 0 || result.Stdout != want || result.Stderr != "" {
		t.Fatalf("result=%+v", result)
	}
}

func TestTypeDiscoveryUsesRegisteredCommandSurface(t *testing.T) {
	shell := newTestBash(t)
	result := shell.Exec(context.Background(), `
type echo ls bash missing; echo status=$?
type -t echo ls bash type missing; echo typed=$?
type -a ls bash
command type -t ls
builtin type ls missing; echo builtin_status=$?
builtin -- type -t echo bash missing; echo builtin_typed=$?
command -v type
`, ExecOptions{})
	wantOut := "echo is a shell builtin\nls is a gash command\nbash is a gash command\nstatus=1\nbuiltin\nfile\nfile\nbuiltin\ntyped=1\nls is a gash command\nbash is a gash command\nfile\nls is a gash command\nbuiltin_status=1\nbuiltin\nfile\nbuiltin_typed=1\ntype\n"
	wantErr := "type: missing: not found\ntype: missing: not found\n"
	if result.ExitCode != 0 || result.Stdout != wantOut || result.Stderr != wantErr {
		t.Fatalf("result=%+v", result)
	}
}

func TestTypeRejectsUnsupportedOptions(t *testing.T) {
	shell := newTestBash(t)
	result := shell.Exec(context.Background(), `type -p ls; echo status=$?`, ExecOptions{})
	if result.ExitCode != 0 || result.Stdout != "status=1\n" || result.Stderr != "type: invalid option -- 'p'\n" {
		t.Fatalf("result=%+v", result)
	}
}

func TestCommandVFindsUname(t *testing.T) {
	shell := newTestBash(t)
	result := shell.Exec(context.Background(), `command -v uname; which uname; uname -a`, ExecOptions{})
	want := "uname\nuname: gash built-in\nGash localhost 1.0.0 #1 gash virtual\n"
	if result.ExitCode != 0 || result.Stdout != want || result.Stderr != "" {
		t.Fatalf("result=%+v", result)
	}
}

func TestUUIDGenDiscoveryAndExecution(t *testing.T) {
	shell := newTestBash(t)
	result := shell.Exec(context.Background(), `command -v uuidgen; which uuidgen; uuidgen`, ExecOptions{})
	if result.ExitCode != 0 || result.Stderr != "" {
		t.Fatalf("result=%+v", result)
	}
	lines := strings.Split(strings.TrimSuffix(result.Stdout, "\n"), "\n")
	if len(lines) != 3 || lines[0] != "uuidgen" || lines[1] != "uuidgen: gash built-in" || len(lines[2]) != 36 {
		t.Fatalf("stdout=%q", result.Stdout)
	}
}

func TestCommandVReflectsOptionalCapabilities(t *testing.T) {
	policy := network.NewPolicy()
	shell, err := New(Options{Network: &policy})
	if err != nil {
		t.Fatal(err)
	}
	result := shell.Exec(context.Background(), `command -v curl`, ExecOptions{})
	if result.ExitCode != 0 || result.Stdout != "curl\n" || result.Stderr != "" {
		t.Fatalf("result=%+v", result)
	}
}

func TestCommandDiscoveryDoesNotCreateFilesystemEntries(t *testing.T) {
	filesystem := fstest.MapFS{".": {Mode: 0o555 | 0x80000000}}
	shell, err := New(Options{FS: filesystem})
	if err != nil {
		t.Fatal(err)
	}
	result := shell.Exec(context.Background(), `test -e /usr/bin/bash; echo test=$?; [[ -e /bin/sh ]]; echo bracket=$?; command -v bash`, ExecOptions{})
	if result.ExitCode != 0 || result.Stdout != "test=1\nbracket=1\nbash\n" || result.Stderr != "" {
		t.Fatalf("result=%+v", result)
	}
}

func TestHashUsesOnlyVirtualCommandDiscovery(t *testing.T) {
	shell := newTestBash(t)
	result := shell.Exec(context.Background(), `
hash
hash -r
hash echo ls
hash definitely-missing; echo missing=$?
hash /bin/echo; echo path=$?
hash -t echo; echo option=$?
`, ExecOptions{})
	wantStdout := "missing=1\npath=1\noption=1\n"
	wantStderr := "hash: definitely-missing: not found\nhash: /bin/echo: not found\nhash: unsupported option: -t\n"
	if result.ExitCode != 0 || result.Stdout != wantStdout || result.Stderr != wantStderr {
		t.Fatalf("result=%+v", result)
	}
}

func TestHashDiscoversCustomCommandsAndDocumentsNoHostPaths(t *testing.T) {
	shell, err := New(Options{Commands: []Command{{Name: "custom", Run: func(_ context.Context, _ []string, _ *CommandContext) int {
		return 0
	}}}})
	if err != nil {
		t.Fatal(err)
	}
	result := shell.Exec(context.Background(), `command -v hash; hash custom; hash --help`, ExecOptions{})
	if result.ExitCode != 0 || result.Stderr != "" {
		t.Fatalf("result=%+v", result)
	}
	if !strings.HasPrefix(result.Stdout, "hash\nhash - validate virtual gash command names") ||
		!strings.Contains(result.Stdout, "No host PATH entries are resolved, cached, or reported.") {
		t.Fatalf("stdout=%q", result.Stdout)
	}
}

func TestCommandVFindsCustomCommands(t *testing.T) {
	shell, err := New(Options{Commands: []Command{{Name: "custom", Run: func(_ context.Context, _ []string, c *CommandContext) int {
		return 0
	}}}})
	if err != nil {
		t.Fatal(err)
	}
	result := shell.Exec(context.Background(), `command -v custom; custom`, ExecOptions{})
	if result.ExitCode != 0 || result.Stdout != "custom\n" || result.Stderr != "" {
		t.Fatalf("result=%+v", result)
	}
}
