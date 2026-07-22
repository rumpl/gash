package gash

import (
	"context"
	"strings"
	"testing"
)

func TestBackgroundJobStatusAndCommandWait(t *testing.T) {
	shell := newTestBash(t)
	result := shell.Exec(context.Background(), `false & p=$!; command wait "$p"; echo status=$? pid=$p`, ExecOptions{})
	if result.ExitCode != 0 || result.Stdout != "status=1 pid=2001\n" || result.Stderr != "" {
		t.Fatalf("result=%+v", result)
	}
}

func TestJobsListsRunningAndCompletedVirtualJobs(t *testing.T) {
	shell := newTestBash(t)
	result := shell.Exec(context.Background(), `sleep 0.05 & p=$!; jobs -p; jobs; wait "$p"; jobs`, ExecOptions{})
	if result.ExitCode != 0 || result.Stderr != "" {
		t.Fatalf("result=%+v", result)
	}
	if !strings.HasPrefix(result.Stdout, "2001\n") || !strings.Contains(result.Stdout, "[1]  Running\tsleep 0.05 &\n") || !strings.Contains(result.Stdout, "[1]  Done\tsleep 0.05 &\n") {
		t.Fatalf("jobs output=%q", result.Stdout)
	}
}

func TestBackgroundInheritsArraysAndIFS(t *testing.T) {
	shell := newTestBash(t)
	result := shell.Exec(context.Background(), `values=(zero one); IFS=:; { printf '%s:%s\n' "${values[1]}" "$IFS"; } & wait`, ExecOptions{})
	if result.ExitCode != 0 || result.Stdout != "one::\n" || result.Stderr != "" {
		t.Fatalf("result=%+v", result)
	}
}

func TestBackgroundTrapChangesDoNotAffectParent(t *testing.T) {
	shell := newTestBash(t)
	result := shell.Exec(context.Background(), `trap 'echo parent' INT; { trap 'echo child' INT; } & wait; kill -INT $$`, ExecOptions{})
	if result.ExitCode != 0 || result.Stdout != "parent\n" || result.Stderr != "" {
		t.Fatalf("result=%+v", result)
	}
}

func TestKillCancelsVirtualBackgroundJob(t *testing.T) {
	shell := newTestBash(t)
	result := shell.Exec(context.Background(), `sleep 1 & p=$!; kill -TERM "$p"; wait "$p"; echo status=$?`, ExecOptions{})
	if result.ExitCode != 0 || result.Stdout != "status=143\n" || result.Stderr != "" {
		t.Fatalf("result=%+v", result)
	}
}

func TestMultipleBackgroundJobsReceiveDistinctPIDs(t *testing.T) {
	shell := newTestBash(t)
	result := shell.Exec(context.Background(), `sleep 0.02 & first=$!; sleep 0.01 & second=$!; echo "$first $second"; wait`, ExecOptions{})
	if result.ExitCode != 0 || result.Stdout != "2001 2002\n" || result.Stderr != "" {
		t.Fatalf("result=%+v", result)
	}
}
