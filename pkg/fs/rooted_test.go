package fs

import (
	"errors"
	iofs "io/fs"
	"strings"
	"testing"
)

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
