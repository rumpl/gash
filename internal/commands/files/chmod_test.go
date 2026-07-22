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

func TestChmodReportsReadOnlyCapabilityError(t *testing.T) {
	filesystem := runCommand(t, commandChmod, nil, map[string]string{"README.md": "docs"}).filesystem
	result := runCommandWithStandardFS(t, commandChmod, []string{"600", "README.md"}, readOnlyTestFS{FS: filesystem})
	if result.exitCode != 1 || result.stdout != "" || result.stderr != "chmod: README.md: filesystem is read-only\n" {
		t.Fatalf("result=%+v", result)
	}
}
