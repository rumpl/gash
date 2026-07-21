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
	result := runCommandWithFS(t, commandReadlink, []string{"link"}, filesystem)
	if result.exitCode != 0 || result.stdout != "a\n" {
		t.Fatalf("exit=%d stdout=%q stderr=%q", result.exitCode, result.stdout, result.stderr)
	}
}
