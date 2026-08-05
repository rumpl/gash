package fs

import (
	"errors"
	iofs "io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRootedAbsoluteVirtualSymlinkRoundTrip(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "work", "dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	filesystem, err := NewRooted(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := filesystem.Symlink("/work/dir", "work/link"); err != nil {
		t.Fatal(err)
	}
	target, err := filesystem.Readlink("work/link")
	if err != nil || target != "/work/dir" {
		t.Fatalf("Readlink = %q, %v", target, err)
	}
	if _, err := filesystem.Stat("work/link"); err != nil {
		t.Fatalf("absolute virtual link is not host-traversable: %v", err)
	}
}

func TestRootedErrorsDoNotExposeHostRoot(t *testing.T) {
	root := t.TempDir()
	filesystem, err := NewRooted(root)
	if err != nil {
		t.Fatal(err)
	}
	_, err = filesystem.ReadFile("no/such/file")
	if err == nil {
		t.Fatal("expected missing file error")
	}
	if strings.Contains(err.Error(), root) {
		t.Fatalf("error exposed host root %q: %v", root, err)
	}
	if !errors.Is(err, iofs.ErrNotExist) {
		t.Fatalf("error does not preserve fs.ErrNotExist: %v", err)
	}
}
