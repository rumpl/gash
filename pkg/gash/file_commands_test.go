package gash

import (
	"context"
	"strings"
	"testing"
)

func TestRecursiveCopyAndHardLinks(t *testing.T) {
	b := newTestBash(t)
	result := b.Exec(context.Background(), "mkdir -p src/sub; echo data > src/sub/a; chmod 600 src/sub/a; cp -rp src dst; stat -c '%a:%s' dst/sub/a", ExecOptions{})
	if result.ExitCode != 0 || result.Stdout != "600:5\n" {
		t.Fatalf("%+v", result)
	}
	result = b.Exec(context.Background(), "ln dst/sub/a linked; echo changed > linked; cat dst/sub/a", ExecOptions{})
	if result.ExitCode != 0 || result.Stdout != "changed\n" {
		t.Fatalf("%+v", result)
	}
}

func TestChmodSymbolicAndRecursive(t *testing.T) {
	b := newTestBash(t)
	result := b.Exec(context.Background(), "mkdir -p d/sub; touch d/a d/sub/b; chmod -R u+x d; stat -c %a d/a d/sub/b", ExecOptions{})
	if result.ExitCode != 0 || result.Stdout != "744\n744\n" {
		t.Fatalf("%+v", result)
	}
}

func TestFileDetection(t *testing.T) {
	b := newTestBash(t)
	_ = b.WriteFile("main.go", "package main\n")
	result := b.Exec(context.Background(), "mkdir dir; ln -s main.go link; file main.go dir link; file -bi main.go", ExecOptions{})
	if result.ExitCode != 0 {
		t.Fatalf("%+v", result)
	}
	for _, want := range []string{"main.go: Go source", "dir: directory", "link: symbolic link to main.go", "text/x-go"} {
		if !strings.Contains(result.Stdout, want) {
			t.Fatalf("missing %q in %q", want, result.Stdout)
		}
	}
}

func TestTreeDuAndSplit(t *testing.T) {
	b := newTestBash(t)
	result := b.Exec(context.Background(), "mkdir -p project/sub; printf 'a\\nb\\nc\\n' > project/input; split -l 2 project/input part-; tree project; cat part-aa part-ab; du -s project", ExecOptions{})
	if result.ExitCode != 0 {
		t.Fatalf("%+v", result)
	}
	for _, want := range []string{"`-- sub", "|-- input", "a\nb\nc\n", "project"} {
		if !strings.Contains(result.Stdout, want) {
			t.Fatalf("missing %q in %q", want, result.Stdout)
		}
	}
}

func TestRmdirOnlyRemovesEmptyDirectories(t *testing.T) {
	b := newTestBash(t)
	result := b.Exec(context.Background(), "mkdir -p a/b; touch a/file; rmdir a", ExecOptions{})
	if result.ExitCode == 0 || !strings.Contains(result.Stderr, "Directory not empty") {
		t.Fatalf("%+v", result)
	}
	result = b.Exec(context.Background(), "rmdir -p a/b", ExecOptions{})
	if result.ExitCode != 0 {
		t.Fatalf("%+v", result)
	}
	result = b.Exec(context.Background(), "test -d a", ExecOptions{})
	if result.ExitCode != 0 {
		t.Fatalf("non-empty parent removed: %+v", result)
	}
}
