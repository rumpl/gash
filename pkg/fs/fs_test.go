package fs

import (
	"errors"
	iofs "io/fs"
	"testing"
	"testing/fstest"
)

func TestMemoryImplementsStandardFS(t *testing.T) {
	m := NewMemory(32)
	if err := m.MkdirAll("home/user", 0755); err != nil {
		t.Fatal(err)
	}
	if err := m.WriteFile("home/user/a", []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := m.AppendFile("home/user/a", []byte(" world"), 0644); err != nil {
		t.Fatal(err)
	}
	got, err := iofs.ReadFile(m, "home/user/a")
	if err != nil || string(got) != "hello world" {
		t.Fatalf("got %q, %v", got, err)
	}
	entries, err := iofs.ReadDir(m, "home/user")
	if err != nil || len(entries) != 1 || entries[0].Name() != "a" {
		t.Fatalf("entries=%v err=%v", entries, err)
	}
	if err := fstest.TestFS(m, "home/user/a"); err != nil {
		t.Fatal(err)
	}
}
func TestMemoryFileLifecycle(t *testing.T) {
	m := NewMemory(32)
	_ = m.MkdirAll("home/user", 0755)
	_ = m.WriteFile("home/user/a", []byte("hello"), 0644)
	if err := m.Rename("home/user/a", "home/user/b"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.ReadFile("home/user/a"); !errors.Is(err, iofs.ErrNotExist) {
		t.Fatalf("old path: %v", err)
	}
}
func TestMemorySymlinksAndQuota(t *testing.T) {
	m := NewMemory(5)
	_ = m.Mkdir("data", 0755)
	_ = m.WriteFile("data/a", []byte("hello"), 0644)
	if err := m.Symlink("a", "data/link"); err != nil {
		t.Fatal(err)
	}
	got, err := m.ReadFile("data/link")
	if err != nil || string(got) != "hello" {
		t.Fatalf("got %q, %v", got, err)
	}
	if err := m.AppendFile("data/a", []byte("!"), 0644); !errors.Is(err, ErrQuota) {
		t.Fatalf("expected quota, got %v", err)
	}
}
func TestMemoryHardLinksAndMetadata(t *testing.T) {
	m := NewMemory(0)
	if err := m.WriteFile("a", []byte("one"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := m.Link("a", "b"); err != nil {
		t.Fatal(err)
	}
	if err := m.AppendFile("b", []byte("two"), 0644); err != nil {
		t.Fatal(err)
	}
	got, _ := m.ReadFile("a")
	if string(got) != "onetwo" {
		t.Fatalf("hard link content=%q", got)
	}
	if err := m.Chmod("a", 0600); err != nil {
		t.Fatal(err)
	}
	info, _ := m.Stat("b")
	if info.Mode().Perm() != 0600 {
		t.Fatalf("mode=%o", info.Mode())
	}
	if err := m.Remove("a"); err != nil {
		t.Fatal(err)
	}
	if m.Used() != 6 {
		t.Fatalf("linked bytes released early: %d", m.Used())
	}
	if err := m.Remove("b"); err != nil {
		t.Fatal(err)
	}
	if m.Used() != 0 {
		t.Fatalf("bytes retained: %d", m.Used())
	}
}
func TestMemoryRecursiveRemove(t *testing.T) {
	m := NewMemory(0)
	_ = m.MkdirAll("a/b", 0755)
	_ = m.WriteFile("a/b/f", []byte("x"), 0644)
	if err := m.Remove("a"); !errors.Is(err, ErrNotEmpty) {
		t.Fatalf("expected not empty, got %v", err)
	}
	if err := m.RemoveAll("a"); err != nil {
		t.Fatal(err)
	}
	if m.Used() != 0 {
		t.Fatalf("used=%d", m.Used())
	}
}
func TestHelpersAcceptAnyStandardFS(t *testing.T) {
	var standard iofs.FS = fstest.MapFS{"hello.txt": {Data: []byte("hello")}}
	got, err := ReadFile(standard, "/hello.txt")
	if err != nil || string(got) != "hello" {
		t.Fatalf("got %q, %v", got, err)
	}
	if err := WriteFile(standard, "/x", nil, 0644); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("expected read-only, got %v", err)
	}
}
