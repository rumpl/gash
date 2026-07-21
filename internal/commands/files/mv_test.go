package files

import (
	"testing"
)

func TestMv(t *testing.T) {
	result := runCommand(t, commandMV, []string{"a", "b"}, map[string]string{"a": "x"})

	if result.exitCode != 0 || !exists(result.filesystem, "work/b") || exists(result.filesystem, "work/a") {
		t.Fatalf("exit=%d stdout=%q stderr=%q", result.exitCode, result.stdout, result.stderr)
	}
}
