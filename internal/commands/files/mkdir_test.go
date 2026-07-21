package files

import (
	"testing"
)

func TestMkdir(t *testing.T) {
	result := runCommand(t, commandMkdir, []string{"new"}, nil)

	if result.exitCode != 0 || !exists(result.filesystem, "work/new") {
		t.Fatalf("exit=%d stdout=%q stderr=%q", result.exitCode, result.stdout, result.stderr)
	}
}
