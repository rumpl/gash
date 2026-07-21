package files

import (
	"testing"
)

func TestStat(t *testing.T) {
	result := runCommand(t, commandStat, []string{"-c", "%s", "a"}, map[string]string{"a": "xyz"})

	if result.exitCode != 0 || result.stdout != "3\n" {
		t.Fatalf("exit=%d stdout=%q stderr=%q", result.exitCode, result.stdout, result.stderr)
	}
}
