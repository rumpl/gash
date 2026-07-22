package fs

import (
	"bytes"
	"errors"
	iofs "io/fs"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"testing/fstest"
)

func TestMemoryBinarySymlinkHardLinkQuotaAndConcurrentAccess(t *testing.T) {
	m := NewMemory(8)
	if err := m.MkdirAll("dir", 0o755); err != nil {
		t.Fatal(err)
	}
	binary := []byte{0, 1, 2, 0xff}
	if err := m.WriteFile("dir/file", binary, 0o640); err != nil {
		t.Fatal(err)
	}
	if err := m.Symlink("file", "dir/sym"); err != nil {
		t.Fatal(err)
	}
	got, err := m.ReadFile("dir/sym")
	if err != nil || !bytes.Equal(got, binary) {
		t.Fatalf("symlink read got %v err=%v", got, err)
	}
	if err := m.Link("dir/file", "dir/hard"); err != nil {
		t.Fatal(err)
	}
	if err := m.AppendFile("dir/hard", []byte{3, 4, 5, 6}, 0o640); err != nil {
		t.Fatal(err)
	}
	if err := m.AppendFile("dir/file", []byte{7}, 0o640); !errors.Is(err, ErrQuota) {
		t.Fatalf("expected quota after hard-link append, got %v", err)
	}
	if err := m.WriteFile("dir/file", []byte{9}, 0o640); err != nil {
		t.Fatal(err)
	}
	if used := m.Used(); used != 1 {
		t.Fatalf("truncate quota used=%d", used)
	}

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 32; j++ {
				_, _ = m.ReadFile("dir/hard")
				_ = m.AppendFile("dir/hard", nil, 0o640)
			}
		}()
	}
	wg.Wait()
}

func TestMemorySymlinkParentsAndTraversalArePredictable(t *testing.T) {
	m := NewMemory(0)
	if err := m.MkdirAll("real/sub", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := m.Symlink("real/sub", "linkdir"); err != nil {
		t.Fatal(err)
	}
	if err := m.WriteFile("linkdir/file", []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := m.ReadFile("real/sub/file")
	if err != nil || string(got) != "ok" {
		t.Fatalf("write through symlink parent got %q err=%v", got, err)
	}
	if _, err := m.ReadFile("../escape"); !errors.Is(err, iofs.ErrInvalid) {
		t.Fatalf("expected invalid traversal, got %v", err)
	}
	if _, err := m.ReadFile("/absolute"); !errors.Is(err, iofs.ErrInvalid) {
		t.Fatalf("expected invalid absolute path, got %v", err)
	}
}

func TestMountableNestedMountShadowingAndCrossDeviceBoundaries(t *testing.T) {
	base := memoryWithFile(t, "mnt/base.txt", "base")
	outer := memoryWithFile(t, "outer.txt", "outer")
	inner := memoryWithFile(t, "inner.txt", "inner")
	m, err := NewMountable(MountableOptions{Base: base})
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Mount("mnt", outer); err != nil {
		t.Fatal(err)
	}
	if err := m.Mount("mnt/nested", inner); err != nil {
		t.Fatal(err)
	}
	got, err := m.ReadFile("mnt/nested/inner.txt")
	if err != nil || string(got) != "inner" {
		t.Fatalf("nested mount got %q err=%v", got, err)
	}
	entries, err := m.ReadDir("mnt")
	if err != nil || names(entries) != "nested,outer.txt" {
		t.Fatalf("shadowed entries=%s err=%v", names(entries), err)
	}
	if err := m.Link("mnt/outer.txt", "mnt/nested/linked"); err == nil || !errors.Is(err, ErrCrossDevice) {
		t.Fatalf("expected cross-device link, got %v", err)
	}
}

func TestOverlayPrefersUpperMergesDirectoriesAndWritesUpper(t *testing.T) {
	upper := memoryWithFile(t, "dir/same", "upper")
	lower := fstest.MapFS{
		"dir/same":  {Data: []byte("lower")},
		"dir/lower": {Data: []byte("lower-only")},
	}
	o, err := NewOverlay(OverlayOptions{Upper: upper, Lower: lower})
	if err != nil {
		t.Fatal(err)
	}
	got, err := o.ReadFile("dir/same")
	if err != nil || string(got) != "upper" {
		t.Fatalf("overlay shadow got %q err=%v", got, err)
	}
	entries, err := o.ReadDir("dir")
	if err != nil || names(entries) != "lower,same" {
		t.Fatalf("merged entries=%s err=%v", names(entries), err)
	}
	if err := o.WriteFile("dir/new", []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := lower.Open("dir/new"); !errors.Is(err, iofs.ErrNotExist) {
		t.Fatalf("lower mutated: %v", err)
	}
	got, _ = upper.ReadFile("dir/new")
	if string(got) != "new" {
		t.Fatalf("upper new=%q", got)
	}
	if err := o.Remove("dir/lower"); !errors.Is(err, iofs.ErrNotExist) {
		t.Fatalf("lower-only remove should not whiteout, got %v", err)
	}
}

func TestRootedPreventsSymlinkTraversalEscapes(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "inside.txt"), []byte("inside"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	r, err := NewRooted(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.ReadFile("escape"); !errors.Is(err, iofs.ErrPermission) {
		t.Fatalf("expected symlink escape denied, got %v", err)
	}
	if err := r.WriteFile("../outside", []byte("x"), 0o644); !errors.Is(err, iofs.ErrInvalid) {
		t.Fatalf("expected traversal denied, got %v", err)
	}
	if err := r.WriteFile("inside.txt", []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(root, "inside.txt"))
	if err != nil || string(got) != "changed" {
		t.Fatalf("root write got %q err=%v", got, err)
	}
}
