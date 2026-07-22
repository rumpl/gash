package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/docker/docker-agent/pkg/tools"
	"github.com/rumpl/gash/pkg/gash"
)

func TestNewShellIsReadOnlyByDefault(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "input.txt")
	if err := os.WriteFile(path, []byte("original\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	shell, _, err := newShell(options{root: root})
	if err != nil {
		t.Fatal(err)
	}
	result := shell.Exec(context.Background(), `printf 'changed\n' > /input.txt`, gash.ExecOptions{})
	if result.ExitCode == 0 {
		t.Fatal("write unexpectedly succeeded")
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "original\n" {
		t.Fatalf("host file changed: %q", contents)
	}
}

func TestWritableFlagExposesMutationCapabilities(t *testing.T) {
	root := t.TempDir()
	shell, _, err := newShell(options{root: root, writable: true})
	if err != nil {
		t.Fatal(err)
	}
	result := shell.Exec(context.Background(), `printf 'created\n' > /output.txt`, gash.ExecOptions{})
	if result.ExitCode != 0 {
		t.Fatalf("result=%+v", result)
	}
	contents, err := os.ReadFile(filepath.Join(root, "output.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "created\n" {
		t.Fatalf("host file=%q", contents)
	}
}

func TestShellHandlerReturnsStructuredResult(t *testing.T) {
	shell, _, err := newShell(options{root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	arguments, err := json.Marshal(shellArgs{Cmd: `printf 'hello\n'`})
	if err != nil {
		t.Fatal(err)
	}
	result, err := shellHandler(shell)(context.Background(), tools.ToolCall{
		Function: tools.FunctionCall{Arguments: string(arguments)},
	}, tools.NopRuntime{})
	if err != nil {
		t.Fatal(err)
	}
	var output shellOutput
	if err := json.Unmarshal([]byte(result.Output), &output); err != nil {
		t.Fatal(err)
	}
	if output.ExitCode != 0 || output.Stdout != "hello\n" || output.Stderr != "" {
		t.Fatalf("output=%+v", output)
	}
}
