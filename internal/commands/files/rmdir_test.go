package files

import (
	"testing"

	gfs "github.com/rumpl/gash/pkg/fs"
)

func TestRmdir(t *testing.T) {
	filesystem := gfs.NewMemory(0)
	if err := filesystem.MkdirAll("work/empty", 0o755); err != nil {
		t.Fatal(err)
	}
	result := runCommandWithFS(t, commandRmdir, []string{"empty"}, filesystem)
	if result.exitCode != 0 || exists(filesystem, "work/empty") {
		t.Fatalf("exit=%d stdout=%q stderr=%q", result.exitCode, result.stdout, result.stderr)
	}
}
