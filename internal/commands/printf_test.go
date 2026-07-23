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

func TestPrintfBDecodesHexadecimalAndOctalEscapes(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cwd := "/"
	ctx := &command.Context{Cwd: &cwd, Stdin: &bytes.Buffer{}, Stdout: &stdout, Stderr: &stderr}
	exitCode := commandPrintf(context.Background(), []string{"%b\\n", `hex=\x41 oct=\0101`}, ctx)
	if exitCode != 0 || stdout.String() != "hex=A oct=A\n" || stderr.String() != "" {
		t.Fatalf("exit=%d stdout=%q stderr=%q", exitCode, stdout.String(), stderr.String())
	}
}

func TestPrintfDecodesUnicodeEscapesInFormatAndBConversion(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cwd := "/"
	ctx := &command.Context{Cwd: &cwd, Stdin: &bytes.Buffer{}, Stdout: &stdout, Stderr: &stderr}
	exitCode := commandPrintf(context.Background(), []string{`\u0041 \U0001F600 %b\n`, `\u0042 \U1F680`}, ctx)
	if exitCode != 0 || stdout.String() != "A 😀 B 🚀\n" || stderr.String() != "" {
		t.Fatalf("exit=%d stdout=%q stderr=%q", exitCode, stdout.String(), stderr.String())
	}
}

func TestPrintfPreservesInvalidUnicodeEscapes(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cwd := "/"
	ctx := &command.Context{Cwd: &cwd, Stdin: &bytes.Buffer{}, Stdout: &stdout, Stderr: &stderr}
	exitCode := commandPrintf(context.Background(), []string{`\uXYZ \U00110000\n`}, ctx)
	if exitCode != 0 || stdout.String() != "\\uXYZ \\U00110000\n" || stderr.String() != "" {
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
