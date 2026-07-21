package files

import (
	"strings"
	"testing"
)

func TestDu(t *testing.T) {
	result := runCommand(t, commandDu, []string{"-s", "."}, map[string]string{"a": "x"})

	if result.exitCode != 0 || !strings.Contains(result.stdout, "\t.") {
		t.Fatalf("exit=%d stdout=%q stderr=%q", result.exitCode, result.stdout, result.stderr)
	}
}
