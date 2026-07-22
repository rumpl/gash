package gash

import (
	"context"
	"testing"
)

func TestBashBasePrefixedArithmetic(t *testing.T) {
	shell := newTestBash(t)
	result := shell.Exec(
		context.Background(),
		`echo $((10#007)); echo $((16#ff)); echo $((64#_)); n=007; echo $((10#$n + 1))`,
		ExecOptions{},
	)
	if result.ExitCode != 0 || result.Stdout != "7\n255\n63\n8\n" || result.Stderr != "" {
		t.Fatalf("result=%+v", result)
	}
}
