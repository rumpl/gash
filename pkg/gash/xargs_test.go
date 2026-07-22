package gash

import (
	"context"
	"strings"
	"testing"
)

func TestXargsThroughShellPipeline(t *testing.T) {
	shell := newTestBash(t)
	result := shell.Exec(context.Background(), `printf 'a b c d e\n' | xargs -n 2 echo`, ExecOptions{})
	if result.ExitCode != 0 || result.Stdout != "a b\nc d\ne\n" || result.Stderr != "" {
		t.Fatalf("result=%+v", result)
	}
}

func TestXargsDispatchesVirtualFileCommands(t *testing.T) {
	shell, err := New(Options{
		Files: map[string]string{
			"/project/data/a.txt": "content-a",
			"/project/data/b.txt": "content-b",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	result := shell.Exec(
		context.Background(),
		`find data -name '*.txt' | sort | xargs cat`,
		ExecOptions{Cwd: "/project"},
	)
	if result.ExitCode != 0 || result.Stdout != "content-acontent-b" || result.Stderr != "" {
		t.Fatalf("result=%+v", result)
	}
}

func TestXargsAttachedMaxArgs(t *testing.T) {
	shell := newTestBash(t)
	result := shell.Exec(context.Background(), `seq 3 | xargs -n1 printf 'n=%s\n'`, ExecOptions{})
	if result.ExitCode != 0 || result.Stdout != "n=1\nn=2\nn=3\n" || result.Stderr != "" {
		t.Fatalf("result=%+v", result)
	}
}

func TestXargsReplacementPreservesUTF8(t *testing.T) {
	shell := newTestBash(t)
	result := shell.Exec(
		context.Background(),
		`printf '한글\ncafé\n漢字\n' | xargs -d '\n' -I {} echo item-{}`,
		ExecOptions{},
	)
	if result.ExitCode != 0 || result.Stdout != "item-한글\nitem-café\nitem-漢字\n" {
		t.Fatalf("result=%+v", result)
	}
}

func TestXargsPrintfUsesShellCompatibleMissingArguments(t *testing.T) {
	shell := newTestBash(t)
	result := shell.Exec(
		context.Background(),
		`printf '%s\n' a b c | xargs -n 2 printf '<%s> <%s>\n'`,
		ExecOptions{},
	)
	if result.ExitCode != 0 || result.Stdout != "<a> <b>\n<c> <>\n" || result.Stderr != "" {
		t.Fatalf("result=%+v", result)
	}
}

func TestXargsPropagatesChildFailure(t *testing.T) {
	shell := newTestBash(t)
	result := shell.Exec(context.Background(), `printf 'missing.txt\n' | xargs cat`, ExecOptions{})
	if result.ExitCode != 1 || !strings.Contains(result.Stderr, "missing.txt") {
		t.Fatalf("result=%+v", result)
	}
}

func TestXargsHelp(t *testing.T) {
	shell := newTestBash(t)
	result := shell.Exec(context.Background(), `xargs --help`, ExecOptions{})
	if result.ExitCode != 0 || !strings.Contains(result.Stdout, "build and execute command lines") {
		t.Fatalf("result=%+v", result)
	}
}
