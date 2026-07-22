package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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

func TestStreamPrinterShowsAssistantToolCallAndResult(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	printer := newStreamPrinter(&stdout, &stderr)
	printer.writeAssistant("I will ")
	printer.writeAssistant("inspect it.")
	printer.writeToolCall(tools.ToolCall{
		Function: tools.FunctionCall{
			Name:      "shell",
			Arguments: `{"cmd":"ls"}`,
		},
	})
	printer.writeToolResult("shell", tools.ResultSuccess(`{"stdout":"README.md\\n","stderr":"","exit_code":0}`), "")
	printer.writeAssistant("The project has a README.")
	printer.finish()

	output := stdout.String()
	for _, expected := range []string{
		"assistant> I will inspect it.",
		"tool call> shell",
		`"cmd": "ls"`,
		"tool result> shell",
		`"exit_code": 0`,
		"assistant> The project has a README.",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("output missing %q:\n%s", expected, output)
		}
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr=%q", stderr.String())
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
