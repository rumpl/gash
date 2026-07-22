package fs

import (
	"errors"
	iofs "io/fs"
	"testing"
)

func TestReadOnlyPreservesSymlinkInspectionCapabilities(t *testing.T) {
	memory := NewMemory(0)
	if err := memory.WriteFile("target", []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := memory.Symlink("target", "link"); err != nil {
		t.Fatal(err)
	}
	view := ReadOnly(memory)
	target, err := Readlink(view, "link")
	if err != nil || target != "target" {
		t.Fatalf("target=%q err=%v", target, err)
	}
	if _, err := Readlink(view, "target"); err == nil || err.Error() != "not a symbolic link" {
		t.Fatalf("regular file error=%v", err)
	}
	info, err := Lstat(view, "link")
	if err != nil || info.Mode()&iofs.ModeSymlink == 0 {
		t.Fatalf("lstat mode=%v err=%v", info.Mode(), err)
	}
}

func TestReadOnlyStripsMutationCapabilities(t *testing.T) {
	memory := NewMemory(0)
	view := ReadOnly(memory)
	if _, ok := view.(WriteFileFS); ok {
		t.Fatal("read-only view exposes WriteFileFS")
	}
	if err := WriteFile(view, "/file", []byte("data"), 0o644); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("err=%v", err)
	}
}
