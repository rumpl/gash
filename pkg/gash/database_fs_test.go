package gash_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	gashfs "github.com/rumpl/gash/pkg/fs"
	"github.com/rumpl/gash/pkg/gash"
)

// TestDatabaseFilesystemDrivesTheShell runs ordinary shell workflows against a
// SQLite-backed filesystem, including a copy mount, and checks that the state
// survives closing and reopening the database.
func TestDatabaseFilesystemDrivesTheShell(t *testing.T) {
	source := t.TempDir()
	if err := os.MkdirAll(filepath.Join(source, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "docs", "notes.md"), []byte("alpha\nbeta\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	database := filepath.Join(t.TempDir(), "shell.db")

	filesystem, err := gashfs.NewSQLite(gashfs.SQLiteOptions{
		Path:   database,
		Mounts: []gashfs.CopyMount{{HostPath: source, Target: "/project"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	shell, err := gash.New(gash.Options{FS: filesystem, Cwd: "/project"})
	if err != nil {
		t.Fatal(err)
	}
	result := shell.Exec(context.Background(), `
		grep beta docs/notes.md
		mkdir -p build
		cp docs/notes.md build/copy.md
		printf 'gamma\n' >> build/copy.md
		wc -l < build/copy.md
	`, gash.ExecOptions{})
	if result.ExitCode != 0 {
		t.Fatalf("exit=%d stderr=%q", result.ExitCode, result.Stderr)
	}
	if result.Stdout != "beta\n3\n" {
		t.Fatalf("stdout=%q", result.Stdout)
	}
	if err := filesystem.Close(); err != nil {
		t.Fatal(err)
	}

	// A second shell over the same database sees everything the first wrote,
	// and the mount is not copied again over the shell's own changes.
	reopened, err := gashfs.NewSQLite(gashfs.SQLiteOptions{
		Path:   database,
		Mounts: []gashfs.CopyMount{{HostPath: source, Target: "/project"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	shell, err = gash.New(gash.Options{FS: reopened, Cwd: "/project"})
	if err != nil {
		t.Fatal(err)
	}
	result = shell.Exec(context.Background(), `find . -type f | sort; cat build/copy.md`, gash.ExecOptions{})
	if result.ExitCode != 0 {
		t.Fatalf("exit=%d stderr=%q", result.ExitCode, result.Stderr)
	}
	want := "./build/copy.md\n./docs/notes.md\nalpha\nbeta\ngamma\n"
	if result.Stdout != want {
		t.Fatalf("stdout=%q want %q", result.Stdout, want)
	}
}

// TestDatabaseFilesystemHostsSQLiteCommand checks that the sqlite3 built-in,
// which reads and writes database bytes through the virtual filesystem, works
// when that filesystem is itself stored in a database.
func TestDatabaseFilesystemHostsSQLiteCommand(t *testing.T) {
	filesystem, err := gashfs.NewSQLite(gashfs.SQLiteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer filesystem.Close()
	if err := filesystem.MkdirAll("data", 0o755); err != nil {
		t.Fatal(err)
	}
	shell, err := gash.New(gash.Options{FS: filesystem, Cwd: "/data"})
	if err != nil {
		t.Fatal(err)
	}
	result := shell.Exec(context.Background(),
		`sqlite3 people.db "CREATE TABLE people(name TEXT); INSERT INTO people VALUES ('Ada');" && sqlite3 people.db "SELECT name FROM people;"`,
		gash.ExecOptions{},
	)
	if result.ExitCode != 0 {
		t.Fatalf("exit=%d stderr=%q", result.ExitCode, result.Stderr)
	}
	if result.Stdout != "Ada\n" {
		t.Fatalf("stdout=%q", result.Stdout)
	}
}
