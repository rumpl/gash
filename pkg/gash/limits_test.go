package gash

import (
	"context"
	"strings"
	"testing"
)

func TestLimitProfiles(t *testing.T) {
	normal, err := resolveLimits(Limits{}, NormalProfile)
	if err != nil {
		t.Fatal(err)
	}
	hardened, err := resolveLimits(Limits{}, HardenedProfile)
	if err != nil {
		t.Fatal(err)
	}
	if hardened.MaxSourceBytes >= normal.MaxSourceBytes || hardened.MaxCommandCount >= normal.MaxCommandCount || hardened.MaxOutputBytes >= normal.MaxOutputBytes {
		t.Fatal("hardened profile is not stricter")
	}
	if _, err := resolveLimits(Limits{}, "unknown"); err == nil {
		t.Fatal("invalid profile accepted")
	}
	if _, err := resolveLimits(Limits{MaxSourceBytes: -1}, NormalProfile); err == nil {
		t.Fatal("negative limit accepted")
	}
}
func TestSourceAndInputLimits(t *testing.T) {
	b, err := New(Options{Limits: Limits{MaxSourceBytes: 4, MaxInputBytes: 3}})
	if err != nil {
		t.Fatal(err)
	}
	result := b.Exec(context.Background(), "echo long", ExecOptions{})
	if result.ExitCode != 126 || !strings.Contains(result.Stderr, "source size") {
		t.Fatalf("%+v", result)
	}
	result = b.Exec(context.Background(), "cat", ExecOptions{Stdin: "four"})
	if result.ExitCode != 126 || !strings.Contains(result.Stderr, "input size") {
		t.Fatalf("%+v", result)
	}
}
func TestOutputBudgetIsAggregate(t *testing.T) {
	b, err := New(Options{Limits: Limits{MaxOutputBytes: 8}})
	if err != nil {
		t.Fatal(err)
	}
	if b.limits.MaxOutputBytes != 8 {
		t.Fatalf("resolved output limit=%d", b.limits.MaxOutputBytes)
	}
	result := b.Exec(context.Background(), "echo 1234; echo 5678 >&2", ExecOptions{})
	if result.ExitCode != 126 {
		t.Fatalf("%+v", result)
	}
	if len(result.Stdout)+len(result.Stderr) > 8 {
		t.Fatalf("output budget exceeded: %+v", result)
	}
}
func TestCommandBudgetIncludesBuiltins(t *testing.T) {
	b, err := New(Options{Limits: Limits{MaxCommandCount: 2}})
	if err != nil {
		t.Fatal(err)
	}
	result := b.Exec(context.Background(), "true; true; true", ExecOptions{})
	if result.ExitCode != 126 || !strings.Contains(result.Stderr, "too many commands") {
		t.Fatalf("%+v", result)
	}
}
