package gash

import (
	"context"
	"testing"
)

func TestArithmeticExpansionErrorStopsExecution(t *testing.T) {
	shell := newTestBash(t)
	for _, script := range []string{
		`echo $((1/0)); echo after`,
		`zero=0; echo $((10/zero)); echo after`,
		`zero=0; echo $((10%zero)); echo after`,
	} {
		result := shell.Exec(context.Background(), script, ExecOptions{})
		if result.ExitCode != 1 || result.Stdout != "" || result.Stderr != "division by zero\n" {
			t.Fatalf("script=%q result=%+v", script, result)
		}
	}
}

func TestValidArithmeticExpansionContinues(t *testing.T) {
	shell := newTestBash(t)
	result := shell.Exec(context.Background(), `echo $((8/2)); echo after`, ExecOptions{})
	if result.ExitCode != 0 || result.Stdout != "4\nafter\n" || result.Stderr != "" {
		t.Fatalf("result=%+v", result)
	}
}
