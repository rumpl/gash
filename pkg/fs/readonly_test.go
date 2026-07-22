package fs

import (
	"errors"
	"testing"
)

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
