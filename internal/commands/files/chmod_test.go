package files

import (
	"testing"
)

func TestChmod(t *testing.T) {
	result := runCommand(t, commandChmod, []string{"600", "a"}, map[string]string{"a": "x"})

	if result.exitCode != 0 || mode(result.filesystem, "work/a") != 0o600 {
		t.Fatalf("exit=%d stdout=%q stderr=%q", result.exitCode, result.stdout, result.stderr)
	}
}
