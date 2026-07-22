package files

import (
	"testing"
)

func TestTouch(t *testing.T) {
	result := runCommand(t, commandTouch, []string{"new"}, nil)

	if result.exitCode != 0 || !exists(result.filesystem, "work/new") {
		t.Fatalf("exit=%d stdout=%q stderr=%q", result.exitCode, result.stdout, result.stderr)
	}
}

func TestTouchExistingReadOnlyFileFails(t *testing.T) {
	filesystem := runCommand(t, commandTouch, nil, map[string]string{"README.md": "docs"}).filesystem
	result := runCommandWithStandardFS(t, commandTouch, []string{"README.md"}, readOnlyTestFS{FS: filesystem})
	if result.exitCode != 1 || result.stdout != "" || result.stderr == "" {
		t.Fatalf("result=%+v", result)
	}
}
