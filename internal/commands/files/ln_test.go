package files

import (
	"testing"
)

func TestLn(t *testing.T) {
	result := runCommand(t, commandLNParity, []string{"a", "b"}, map[string]string{"a": "x"})

	if result.exitCode != 0 || !exists(result.filesystem, "work/b") {
		t.Fatalf("exit=%d stdout=%q stderr=%q", result.exitCode, result.stdout, result.stderr)
	}
}
