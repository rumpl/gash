package fs

import (
	"errors"
	iofs "io/fs"
	"testing"
	"testing/fstest"
)

func memoryWithFile(t *testing.T, name, content string) *Memory {
	t.Helper()
	m := NewMemory(0)
	if dir := parent(name); dir != "." {
		if err := m.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := m.WriteFile(name, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestMountableRoutesAndExposesVirtualDirectories(t *testing.T) {
	base := memoryWithFile(t, "base.txt", "base")
	knowledge := fstest.MapFS{"docs/readme.txt": {Data: []byte("mounted")}}
	m, err := NewMountable(MountableOptions{Base: base, Mounts: []MountConfig{{Point: "/mnt/knowledge", FS: knowledge}}})
	if err != nil {
		t.Fatal(err)
	}
	got, err := iofs.ReadFile(m, "mnt/knowledge/docs/readme.txt")
	if err != nil || string(got) != "mounted" {
		t.Fatalf("got %q, %v", got, err)
	}
	entries, err := iofs.ReadDir(m, ".")
	if err != nil {
		t.Fatal(err)
	}
	if names(entries) != "base.txt,mnt" {
		t.Fatalf("root entries: %s", names(entries))
	}
	entries, err = iofs.ReadDir(m, "mnt")
	if err != nil || names(entries) != "knowledge" {
		t.Fatalf("mount parent: %s, %v", names(entries), err)
	}
	if err := fstest.TestFS(m, "base.txt", "mnt/knowledge/docs/readme.txt"); err != nil {
		t.Fatal(err)
	}
}

func TestMountableForwardsWriteCapabilities(t *testing.T) {
	base := NewMemory(0)
	workspace := NewMemory(0)
	m, err := NewMountable(MountableOptions{Base: base, Mounts: []MountConfig{{Point: "workspace", FS: workspace}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := m.WriteFile("workspace/a.txt", []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := workspace.ReadFile("a.txt")
	if err != nil || string(got) != "a" {
		t.Fatalf("got %q, %v", got, err)
	}
	if err := m.WriteFile("root.txt", []byte("root"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := base.ReadFile("root.txt"); err != nil {
		t.Fatal(err)
	}
}

func TestMountableReadOnlyMount(t *testing.T) {
	m, err := NewMountable(MountableOptions{Mounts: []MountConfig{{Point: "readonly", FS: fstest.MapFS{"a": {Data: []byte("a")}}}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := m.WriteFile("readonly/a", []byte("x"), 0o644); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("expected read-only, got %v", err)
	}
	if err := m.Remove("readonly"); !errors.Is(err, ErrBusy) {
		t.Fatalf("expected busy, got %v", err)
	}
}

func TestMountableCrossMountRename(t *testing.T) {
	left := memoryWithFile(t, "dir/a", "content")
	right := NewMemory(0)
	m, err := NewMountable(MountableOptions{Mounts: []MountConfig{{Point: "left", FS: left}, {Point: "right", FS: right}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Rename("left/dir", "right/moved"); err != nil {
		t.Fatal(err)
	}
	got, err := right.ReadFile("moved/a")
	if err != nil || string(got) != "content" {
		t.Fatalf("got %q, %v", got, err)
	}
	if _, err := left.Stat("dir"); !errors.Is(err, iofs.ErrNotExist) {
		t.Fatalf("source remains: %v", err)
	}
}

func TestMountableRejectsNestedMounts(t *testing.T) {
	m, _ := NewMountable(MountableOptions{})
	if err := m.Mount("a", NewMemory(0)); err != nil {
		t.Fatal(err)
	}
	if err := m.Mount("a/b", NewMemory(0)); err == nil {
		t.Fatal("nested mount accepted")
	}
	if err := m.Mount(".", NewMemory(0)); err == nil {
		t.Fatal("root mount accepted")
	}
}

func names(entries []iofs.DirEntry) string {
	out := ""
	for i, e := range entries {
		if i > 0 {
			out += ","
		}
		out += e.Name()
	}
	return out
}
