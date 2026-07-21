package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunWithHostRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "message.txt"), []byte("from host\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := run(
		[]string{"--root", root, "-c", "pwd; cat message.txt"},
		&bytes.Buffer{},
		&stdout,
		&stderr,
	)
	if exitCode != 0 {
		t.Fatalf("exit=%d stderr=%q", exitCode, stderr.String())
	}
	if stdout.String() != "/\nfrom host\n" {
		t.Fatalf("stdout=%q", stdout.String())
	}
}

func TestRunRejectsNonDirectoryRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(root, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := run(
		[]string{"--root", root, "-c", "true"},
		&bytes.Buffer{},
		&stdout,
		&stderr,
	)
	if exitCode != 1 {
		t.Fatalf("exit=%d", exitCode)
	}
	if !strings.Contains(stderr.String(), "is not a directory") {
		t.Fatalf("stderr=%q", stderr.String())
	}
}

func TestRunWithoutRootUsesMemoryFilesystem(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := run(
		[]string{"-c", "pwd"},
		&bytes.Buffer{},
		&stdout,
		&stderr,
	)
	if exitCode != 0 {
		t.Fatalf("exit=%d stderr=%q", exitCode, stderr.String())
	}
	if stdout.String() != "/home/user\n" {
		t.Fatalf("stdout=%q", stdout.String())
	}
}
