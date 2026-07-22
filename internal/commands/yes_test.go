package commands

import (
	"bytes"
	"context"
	"testing"
)

type cancelWriter struct {
	bytes.Buffer
	cancel context.CancelFunc
	writes int
}

func (writer *cancelWriter) Write(data []byte) (int, error) {
	writer.writes++
	count, err := writer.Buffer.Write(data)
	if writer.writes == 3 {
		writer.cancel()
	}
	return count, err
}

func TestYesRepeatsArgumentsUntilCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	stdout := &cancelWriter{cancel: cancel}
	commandCtx := &CommandContext{Stdout: stdout, Stderr: &bytes.Buffer{}}
	code := commandYes(ctx, []string{"hello", "world"}, commandCtx)
	if code != 124 || stdout.String() != "hello world\nhello world\nhello world\n" {
		t.Fatalf("code=%d stdout=%q", code, stdout.String())
	}
}
