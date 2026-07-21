package files

import (
	"testing"
)

func TestSplit(t *testing.T) {
	result := runCommand(t, commandSplit, []string{"-b", "1", "a", "part"}, map[string]string{"a": "xy"})

	if result.exitCode != 0 || !exists(result.filesystem, "work/partaa") || !exists(result.filesystem, "work/partab") {
		t.Fatalf("exit=%d stdout=%q stderr=%q", result.exitCode, result.stdout, result.stderr)
	}
}
