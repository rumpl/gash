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

func TestRunWithHostRootDoesNotPermitWrites(t *testing.T) {
	root := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := run(
		[]string{"--root", root, "-c", "echo before; echo data > forbidden; echo after; exit 0"},
		&bytes.Buffer{},
		&stdout,
		&stderr,
	)
	if exitCode != 0 || stdout.String() != "before\nafter\n" || !strings.Contains(stderr.String(), "filesystem is read-only") {
		t.Fatalf("exit=%d stdout=%q stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	if _, err := os.Stat(filepath.Join(root, "forbidden")); !os.IsNotExist(err) {
		t.Fatalf("host file was created: %v", err)
	}
}

func TestRunForwardsScriptFileArgs(t *testing.T) {
	root := t.TempDir()
	script := filepath.Join(root, "script.sh")
	if err := os.WriteFile(script, []byte(`printf '%s/%s/%s\n' "$0" "$1" "$2"`), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := run([]string{script, "left", "right"}, &bytes.Buffer{}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("exit=%d stderr=%q", exitCode, stderr.String())
	}
	want := script + "/left/right\n"
	if stdout.String() != want {
		t.Fatalf("stdout=%q want %q", stdout.String(), want)
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
