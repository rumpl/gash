package fs

import (
	iofs "io/fs"
	"testing"
)

func TestDevNullExistsWithoutBackingFilesystemEntry(t *testing.T) {
	filesystem := NewMemory(0)
	data, err := ReadFile(filesystem, "/dev/null")
	if err != nil || len(data) != 0 {
		t.Fatalf("data=%q err=%v", data, err)
	}
	info, err := Stat(filesystem, "/dev/null")
	if err != nil || info.Name() != "null" || info.IsDir() || info.Mode()&iofs.ModeCharDevice == 0 {
		t.Fatalf("info=%v err=%v", info, err)
	}
	if err := WriteFile(filesystem, "/dev/null", []byte("discarded"), 0o644); err != nil {
		t.Fatal(err)
	}
	data, err = ReadFile(filesystem, "/dev/null")
	if err != nil || len(data) != 0 {
		t.Fatalf("data=%q err=%v", data, err)
	}
}
