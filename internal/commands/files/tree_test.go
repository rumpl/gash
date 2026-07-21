package files

import (
	"strings"
	"testing"
)

func TestTree(t *testing.T) {
	result := runCommand(t, commandTree, []string{"."}, map[string]string{"a": "x"})

	if result.exitCode != 0 || !strings.Contains(result.stdout, "`-- a") {
		t.Fatalf("exit=%d stdout=%q stderr=%q", result.exitCode, result.stdout, result.stderr)
	}
}
