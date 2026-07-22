package files

import (
	"testing"

	gfs "github.com/rumpl/gash/pkg/fs"
)

func TestReadlink(t *testing.T) {
	filesystem := gfs.NewMemory(0)
	if err := filesystem.MkdirAll("work", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := filesystem.Symlink("a", "work/link"); err != nil {
		t.Fatal(err)
	}
	result := runCommandWithStandardFS(t, commandReadlink, []string{"link"}, gfs.ReadOnly(filesystem))
	if result.exitCode != 0 || result.stdout != "a\n" {
		t.Fatalf("exit=%d stdout=%q stderr=%q", result.exitCode, result.stdout, result.stderr)
	}
	if err := filesystem.WriteFile("work/regular", []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	result = runCommandWithStandardFS(t, commandReadlink, []string{"regular"}, gfs.ReadOnly(filesystem))
	if result.exitCode != 1 || result.stderr != "readlink: not a symbolic link\n" {
		t.Fatalf("exit=%d stdout=%q stderr=%q", result.exitCode, result.stdout, result.stderr)
	}
}
