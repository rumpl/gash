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

func TestRmForceOnlySuppressesMissingFiles(t *testing.T) {
	filesystem := runCommand(t, commandRM, nil, map[string]string{"README.md": "docs"}).filesystem
	readOnly := readOnlyTestFS{FS: filesystem}
	result := runCommandWithStandardFS(t, commandRM, []string{"-f", "README.md"}, readOnly)
	if result.exitCode != 1 || result.stderr == "" || !exists(filesystem, "work/README.md") {
		t.Fatalf("existing result=%+v", result)
	}
	result = runCommandWithStandardFS(t, commandRM, []string{"-f", "missing"}, readOnly)
	if result.exitCode != 0 || result.stdout != "" || result.stderr != "" {
		t.Fatalf("missing result=%+v", result)
	}
}
