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
	want := "curl=1\nrealpath=1\nls\nid\necho\n"
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

func TestVerboseCommandAndTypeDiscoveryAvoidFilesystemPaths(t *testing.T) {
	shell := newTestBash(t)
	result := shell.Exec(context.Background(), `command -V bash; type bash; type -a ls`, ExecOptions{})
	want := "bash is a gash command\nbash is a gash command\nls is a gash command\n"
	if result.ExitCode != 0 || result.Stdout != want || result.Stderr != "" {
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
