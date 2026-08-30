package fs

import (
	"errors"
	iofs "io/fs"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

func hostTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src", "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "nested", "run.sh"), []byte("echo hi\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("main.go", filepath.Join(root, "src", "link.go")); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestCopyMountCopiesHostDirectory(t *testing.T) {
	root := hostTree(t)
	filesystem, err := NewSQLite(SQLiteOptions{Mounts: []CopyMount{{HostPath: root, Target: "/workspace"}}})
	if err != nil {
		t.Fatal(err)
	}
	defer filesystem.Close()

	got, err := filesystem.ReadFile("workspace/src/main.go")
	if err != nil || string(got) != "package main\n" {
		t.Fatalf("got %q, %v", got, err)
	}
	info, err := filesystem.Stat("workspace/src/nested/run.sh")
	if err != nil || info.Mode().Perm() != 0o755 {
		t.Fatalf("mode=%v err=%v", info, err)
	}
	target, err := filesystem.Readlink("workspace/src/link.go")
	if err != nil || target != "main.go" {
		t.Fatalf("target=%q err=%v", target, err)
	}

	// The copy is a copy: the source may disappear without the filesystem
	// losing anything.
	if err := os.RemoveAll(root); err != nil {
		t.Fatal(err)
	}
	if got, err := filesystem.ReadFile("workspace/src/link.go"); err != nil || string(got) != "package main\n" {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestCopyMountPolicies(t *testing.T) {
	root := hostTree(t)
	path := filepath.Join(t.TempDir(), "gash.db")
	mount := CopyMount{HostPath: root, Target: "/workspace"}

	first, err := NewSQLite(SQLiteOptions{Path: path, Mounts: []CopyMount{mount}})
	if err != nil {
		t.Fatal(err)
	}
	if err := first.WriteFile("workspace/src/main.go", []byte("edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := first.WriteFile("workspace/scratch.txt", []byte("kept\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	// MountIfAbsent leaves a populated target alone.
	second, err := NewSQLite(SQLiteOptions{Path: path, Mounts: []CopyMount{mount}})
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := second.ReadFile("workspace/src/main.go"); string(got) != "edited\n" {
		t.Fatalf("if-absent overwrote the target: %q", got)
	}
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}

	// MountMerge refreshes source files and keeps everything else.
	merge := mount
	merge.Policy = MountMerge
	third, err := NewSQLite(SQLiteOptions{Path: path, Mounts: []CopyMount{merge}})
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := third.ReadFile("workspace/src/main.go"); string(got) != "package main\n" {
		t.Fatalf("merge did not refresh the file: %q", got)
	}
	if got, _ := third.ReadFile("workspace/scratch.txt"); string(got) != "kept\n" {
		t.Fatalf("merge dropped an unrelated file: %q", got)
	}
	if err := third.Close(); err != nil {
		t.Fatal(err)
	}

	// MountReplace starts from a clean target.
	replace := mount
	replace.Policy = MountReplace
	fourth, err := NewSQLite(SQLiteOptions{Path: path, Mounts: []CopyMount{replace}})
	if err != nil {
		t.Fatal(err)
	}
	defer fourth.Close()
	if _, err := fourth.Stat("workspace/scratch.txt"); !errors.Is(err, iofs.ErrNotExist) {
		t.Fatalf("replace kept an unrelated file: %v", err)
	}
	if got, _ := fourth.ReadFile("workspace/src/main.go"); string(got) != "package main\n" {
		t.Fatalf("replace did not copy the source: %q", got)
	}
}

func TestCopyMountAtRootAndUnknownPolicy(t *testing.T) {
	filesystem := NewMemory(0)
	source := fstest.MapFS{
		"data/users.csv": &fstest.MapFile{Data: []byte("id,name\n"), Mode: 0o644},
		"readme.md":      &fstest.MapFile{Data: []byte("hello\n"), Mode: 0o644},
	}
	if err := ApplyMount(filesystem, CopyMount{Source: source}); err != nil {
		t.Fatal(err)
	}
	if got, err := filesystem.ReadFile("data/users.csv"); err != nil || string(got) != "id,name\n" {
		t.Fatalf("got %q, %v", got, err)
	}
	// The root is populated now, so an if-absent mount is a no-op.
	if err := ApplyMount(filesystem, CopyMount{Source: fstest.MapFS{"other": &fstest.MapFile{Data: []byte("x")}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := filesystem.Stat("other"); !errors.Is(err, iofs.ErrNotExist) {
		t.Fatalf("if-absent copied onto a populated root: %v", err)
	}
	if err := ApplyMount(filesystem, CopyMount{Source: source, Policy: MountPolicy(42)}); err == nil {
		t.Fatal("expected an unknown policy to be rejected")
	}
	if err := ApplyMount(filesystem, CopyMount{}); err == nil {
		t.Fatal("expected a mount without a source to be rejected")
	}
}

func TestCopyMountReplaceClearsRoot(t *testing.T) {
	filesystem := NewMemory(0)
	if err := filesystem.WriteFile("stale.txt", []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	source := fstest.MapFS{"fresh.txt": &fstest.MapFile{Data: []byte("new"), Mode: 0o644}}
	if err := ApplyMount(filesystem, CopyMount{Source: source, Policy: MountReplace}); err != nil {
		t.Fatal(err)
	}
	if _, err := filesystem.Stat("stale.txt"); !errors.Is(err, iofs.ErrNotExist) {
		t.Fatalf("replace kept a stale entry: %v", err)
	}
	if got, err := filesystem.ReadFile("fresh.txt"); err != nil || string(got) != "new" {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestCopyTreeIntoReadOnlyFilesystemFails(t *testing.T) {
	source := fstest.MapFS{"a.txt": &fstest.MapFile{Data: []byte("a")}}
	if err := CopyTree(ReadOnly(NewMemory(0)), "/", source); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("expected read-only, got %v", err)
	}
}
