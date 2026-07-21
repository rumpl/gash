package fs

import (
	"errors"
	"io/fs"
	"testing"
)

func TestMemoryFileLifecycle(t *testing.T) {
	m := NewMemory(32)
	if err := m.Mkdir("/home/user", 0755, true); err != nil {
		t.Fatal(err)
	}
	if err := m.WriteFile("/home/user/a", []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := m.AppendFile("/home/user/a", []byte(" world")); err != nil {
		t.Fatal(err)
	}
	got, err := m.ReadFile("/home/user/a")
	if err != nil || string(got) != "hello world" {
		t.Fatalf("got %q, %v", got, err)
	}
	if err := m.Rename("/home/user/a", "/home/user/b"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.ReadFile("/home/user/a"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("old path: %v", err)
	}
}

func TestMemorySymlinksAndQuota(t *testing.T) {
	m := NewMemory(5)
	_ = m.Mkdir("/data", 0755, false)
	_ = m.WriteFile("/data/a", []byte("hello"), 0644)
	if err := m.Symlink("a", "/data/link"); err != nil {
		t.Fatal(err)
	}
	got, err := m.ReadFile("/data/link")
	if err != nil || string(got) != "hello" {
		t.Fatalf("got %q, %v", got, err)
	}
	if err := m.AppendFile("/data/a", []byte("!")); !errors.Is(err, ErrQuota) {
		t.Fatalf("expected quota, got %v", err)
	}
}

func TestMemoryRecursiveRemove(t *testing.T) {
	m := NewMemory(0)
	_ = m.Mkdir("/a/b", 0755, true)
	_ = m.WriteFile("/a/b/f", []byte("x"), 0644)
	if err := m.Remove("/a", false); !errors.Is(err, ErrNotEmpty) {
		t.Fatalf("expected not empty, got %v", err)
	}
	if err := m.Remove("/a", true); err != nil {
		t.Fatal(err)
	}
	if m.Used() != 0 {
		t.Fatalf("used=%d", m.Used())
	}
}
