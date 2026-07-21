package files

import (
	iofs "io/fs"
	"strings"
	"testing"

	gfs "github.com/rumpl/gash/pkg/fs"
)

func TestFindBasicPredicatesAndDepth(t *testing.T) {
	fsys := gfs.NewMemory(0)
	mustMkdirAll(t, fsys, "work/project/sub/emptydir")
	mustWrite(t, fsys, "work/project/a.txt", "alpha", 0o644)
	mustWrite(t, fsys, "work/project/sub/b.log", "bravo", 0o600)
	mustWrite(t, fsys, "work/project/sub/c.TXT", "", 0o755)

	result := runCommandWithFS(t, commandFind, []string{"project", "-name", "*.txt"}, fsys)
	if result.exitCode != 0 || result.stdout != "project/a.txt\n" {
		t.Fatalf("exit=%d stdout=%q stderr=%q", result.exitCode, result.stdout, result.stderr)
	}

	result = runCommandWithFS(t, commandFind, []string{"project", "-iname", "*.txt", "-type", "f"}, fsys)
	if result.exitCode != 0 || result.stdout != "project/a.txt\nproject/sub/c.TXT\n" {
		t.Fatalf("exit=%d stdout=%q stderr=%q", result.exitCode, result.stdout, result.stderr)
	}

	result = runCommandWithFS(t, commandFind, []string{"project", "-maxdepth", "1", "-type", "f"}, fsys)
	if result.exitCode != 0 || result.stdout != "project/a.txt\n" {
		t.Fatalf("exit=%d stdout=%q stderr=%q", result.exitCode, result.stdout, result.stderr)
	}

	result = runCommandWithFS(t, commandFind, []string{"project", "-mindepth", "2", "-empty"}, fsys)
	if result.exitCode != 0 || result.stdout != "project/sub/c.TXT\nproject/sub/emptydir\n" {
		t.Fatalf("exit=%d stdout=%q stderr=%q", result.exitCode, result.stdout, result.stderr)
	}
}

func TestFindOperatorsPruneAndPrintf(t *testing.T) {
	fsys := gfs.NewMemory(0)
	mustMkdirAll(t, fsys, "work/project/vendor")
	mustWrite(t, fsys, "work/project/main.go", "package main\n", 0o644)
	mustWrite(t, fsys, "work/project/vendor/skip.go", "package skip\n", 0o644)
	mustWrite(t, fsys, "work/project/readme.md", "docs\n", 0o644)

	result := runCommandWithFS(t, commandFind, []string{"project", "-path", "*/vendor", "-prune", "-o", "-name", "*.go", "-print"}, fsys)
	if result.exitCode != 0 || result.stdout != "project/main.go\n" {
		t.Fatalf("exit=%d stdout=%q stderr=%q", result.exitCode, result.stdout, result.stderr)
	}

	result = runCommandWithFS(t, commandFind, []string{"project", "(", "-name", "*.go", "-o", "-name", "*.md", ")", "-printf", "%P:%f:%s:%m\n"}, fsys)
	if result.exitCode != 0 {
		t.Fatalf("exit=%d stdout=%q stderr=%q", result.exitCode, result.stdout, result.stderr)
	}
	for _, want := range []string{"main.go:main.go:13:644\n", "readme.md:readme.md:5:644\n", "vendor/skip.go:skip.go:13:644\n"} {
		if !strings.Contains(result.stdout, want) {
			t.Fatalf("missing %q in %q", want, result.stdout)
		}
	}
}

func TestFindPermDeleteAndFailClosedExec(t *testing.T) {
	fsys := gfs.NewMemory(0)
	mustMkdirAll(t, fsys, "work/project/tmp")
	mustWrite(t, fsys, "work/project/run.sh", "#!/bin/sh\n", 0o755)
	mustWrite(t, fsys, "work/project/tmp/delete.me", "x", 0o644)
	mustWrite(t, fsys, "work/project/keep.txt", "x", 0o644)

	result := runCommandWithFS(t, commandFind, []string{"project", "-perm", "-111", "-type", "f"}, fsys)
	if result.exitCode != 0 || result.stdout != "project/run.sh\n" {
		t.Fatalf("exit=%d stdout=%q stderr=%q", result.exitCode, result.stdout, result.stderr)
	}

	result = runCommandWithFS(t, commandFind, []string{"project/tmp", "-type", "f", "-delete"}, fsys)
	if result.exitCode != 0 || exists(fsys, "work/project/tmp/delete.me") {
		t.Fatalf("exit=%d stdout=%q stderr=%q", result.exitCode, result.stdout, result.stderr)
	}

	result = runCommandWithFS(t, commandFind, []string{"project", "-exec", "echo", "{}", ";"}, fsys)
	if result.exitCode == 0 || !strings.Contains(result.stderr, "-exec is not supported") {
		t.Fatalf("exit=%d stdout=%q stderr=%q", result.exitCode, result.stdout, result.stderr)
	}
}

func TestFindParseErrorsAreFailClosed(t *testing.T) {
	fsys := gfs.NewMemory(0)
	mustMkdirAll(t, fsys, "work/project")
	mustWrite(t, fsys, "work/project/keep.txt", "x", 0o644)

	result := runCommandWithFS(t, commandFind, []string{"project", "-maxdepth", "nope", "-delete"}, fsys)
	if result.exitCode == 0 || !exists(fsys, "work/project/keep.txt") || !strings.Contains(result.stderr, "invalid argument `nope'") {
		t.Fatalf("exit=%d stdout=%q stderr=%q", result.exitCode, result.stdout, result.stderr)
	}

	result = runCommandWithFS(t, commandFind, []string{"project", "!", "-delete"}, fsys)
	if result.exitCode == 0 || !strings.Contains(result.stderr, "refusing to evaluate `-delete' under negation") {
		t.Fatalf("exit=%d stdout=%q stderr=%q", result.exitCode, result.stdout, result.stderr)
	}
}

func mustMkdirAll(t *testing.T, fsys *gfs.Memory, name string) {
	t.Helper()
	if err := fsys.MkdirAll(name, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustWrite(t *testing.T, fsys *gfs.Memory, name, content string, perm iofs.FileMode) {
	t.Helper()
	if err := fsys.WriteFile(name, []byte(content), perm); err != nil {
		t.Fatal(err)
	}
}
