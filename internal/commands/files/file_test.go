package files

import (
	"testing"
)

func TestFile(t *testing.T) {
	result := runCommand(t, commandFile, []string{"a.txt"}, map[string]string{"a.txt": "text"})

	if result.exitCode != 0 || result.stdout != "a.txt: ASCII text\n" {
		t.Fatalf("exit=%d stdout=%q stderr=%q", result.exitCode, result.stdout, result.stderr)
	}
}
