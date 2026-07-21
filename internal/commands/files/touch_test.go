package files

import (
	"testing"
)

func TestTouch(t *testing.T) {
	result := runCommand(t, commandTouch, []string{"new"}, nil)

	if result.exitCode != 0 || !exists(result.filesystem, "work/new") {
		t.Fatalf("exit=%d stdout=%q stderr=%q", result.exitCode, result.stdout, result.stderr)
	}
}
