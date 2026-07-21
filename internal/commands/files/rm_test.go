package files

import (
	"testing"
)

func TestRm(t *testing.T) {
	result := runCommand(t, commandRM, []string{"a"}, map[string]string{"a": "x"})

	if result.exitCode != 0 || exists(result.filesystem, "work/a") {
		t.Fatalf("exit=%d stdout=%q stderr=%q", result.exitCode, result.stdout, result.stderr)
	}
}
