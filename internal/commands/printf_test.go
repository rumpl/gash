package commands

import (
	"bytes"
	"context"
	"testing"

	"github.com/rumpl/gash/internal/command"
)

func TestPrintfUsesEmptyValuesForMissingArguments(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cwd := "/"
	ctx := &command.Context{Cwd: &cwd, Stdin: &bytes.Buffer{}, Stdout: &stdout, Stderr: &stderr}
	exitCode := commandPrintf(context.Background(), []string{"<%s> <%s>\\n", "c"}, ctx)
	if exitCode != 0 || stdout.String() != "<c> <>\n" || stderr.String() != "" {
		t.Fatalf("exit=%d stdout=%q stderr=%q", exitCode, stdout.String(), stderr.String())
	}
}

func TestPrintfQProducesReusableShellWords(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cwd := "/"
	ctx := &command.Context{Cwd: &cwd, Stdin: &bytes.Buffer{}, Stdout: &stdout, Stderr: &stderr}
	exitCode := commandPrintf(context.Background(), []string{"%q\\n", "hello world", "", "plain"}, ctx)
	if exitCode != 0 || stdout.String() != "hello\\ world\n''\nplain\n" || stderr.String() != "" {
		t.Fatalf("exit=%d stdout=%q stderr=%q", exitCode, stdout.String(), stderr.String())
	}
}

func TestPrintfReusesFormatForRemainingArguments(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cwd := "/"
	ctx := &command.Context{Cwd: &cwd, Stdin: &bytes.Buffer{}, Stdout: &stdout, Stderr: &stderr}
	exitCode := commandPrintf(context.Background(), []string{"[%s]\\n", "a", "b"}, ctx)
	if exitCode != 0 || stdout.String() != "[a]\n[b]\n" || stderr.String() != "" {
		t.Fatalf("exit=%d stdout=%q stderr=%q", exitCode, stdout.String(), stderr.String())
	}
}
