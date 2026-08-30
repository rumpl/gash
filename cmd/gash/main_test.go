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

func TestRunForwardsStdinWithCommand(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := run(
		[]string{"-c", "cksum"},
		strings.NewReader("123"+"456"+"789"),
		&stdout,
		&stderr,
	)
	if exitCode != 0 {
		t.Fatalf("exit=%d stderr=%q", exitCode, stderr.String())
	}
	if stdout.String() != "930766865 9\n" {
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

func TestRunWithSQLiteFilesystemPersistsAndMounts(t *testing.T) {
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "seed.txt"), []byte("seeded\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	database := filepath.Join(t.TempDir(), "gash.db")

	var stdout, stderr bytes.Buffer
	exitCode := run(
		[]string{"--sqlite", database, "--mount", source + ":/data", "-c", "cat /data/seed.txt; echo written > /data/out.txt"},
		&bytes.Buffer{},
		&stdout,
		&stderr,
	)
	if exitCode != 0 || stdout.String() != "seeded\n" {
		t.Fatalf("exit=%d stdout=%q stderr=%q", exitCode, stdout.String(), stderr.String())
	}

	// The mounted directory was copied, so removing it changes nothing, and
	// files written by the shell survive in the database.
	if err := os.RemoveAll(source); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	exitCode = run(
		[]string{"--sqlite", database, "-c", "cat /data/seed.txt /data/out.txt"},
		&bytes.Buffer{},
		&stdout,
		&stderr,
	)
	if exitCode != 0 || stdout.String() != "seeded\nwritten\n" {
		t.Fatalf("exit=%d stdout=%q stderr=%q", exitCode, stdout.String(), stderr.String())
	}
}

func TestRunRejectsInvalidDatabaseCombinations(t *testing.T) {
	cases := map[string][]string{
		"--mount requires":   {"--mount", t.TempDir(), "-c", "true"},
		"mutually exclusive": {"--sqlite", filepath.Join(t.TempDir(), "a.db"), "--turso", "libsql://example.turso.io", "-c", "true"},
		"--root cannot be":   {"--root", t.TempDir(), "--sqlite", filepath.Join(t.TempDir(), "b.db"), "-c", "true"},
		"mount policy must":  {"--sqlite", filepath.Join(t.TempDir(), "c.db"), "--mount-policy", "sometimes", "-c", "true"},
	}
	for want, args := range cases {
		var stdout, stderr bytes.Buffer
		if exitCode := run(args, &bytes.Buffer{}, &stdout, &stderr); exitCode != 1 {
			t.Fatalf("%s: exit=%d stderr=%q", want, exitCode, stderr.String())
		}
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("stderr=%q want %q", stderr.String(), want)
		}
	}
}

func TestMountFlagParsing(t *testing.T) {
	var mounts mountFlags
	if err := mounts.Set("/host/dir"); err != nil {
		t.Fatal(err)
	}
	if err := mounts.Set("/host/other:/target"); err != nil {
		t.Fatal(err)
	}
	if len(mounts) != 2 || mounts[0].Target != "/" || mounts[1].HostPath != "/host/other" || mounts[1].Target != "/target" {
		t.Fatalf("mounts=%+v", mounts)
	}
	if err := mounts.Set(":/target"); err == nil {
		t.Fatal("expected an empty host path to be rejected")
	}
	if err := mounts.Set("/host/dir:relative"); err == nil {
		t.Fatal("expected a relative target to be rejected")
	}
}
