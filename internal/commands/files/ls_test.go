package files

import (
	"testing"
)

func TestLs(t *testing.T) {
	result := runCommand(t, commandLS, []string{"."}, map[string]string{"a": "x"})

	if result.exitCode != 0 || result.stdout != "a\n" {
		t.Fatalf("exit=%d stdout=%q stderr=%q", result.exitCode, result.stdout, result.stderr)
	}
}
