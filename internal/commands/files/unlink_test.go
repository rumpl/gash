package files

import (
	"strings"
	"testing"
)

func TestUnlinkRemovesExactlyOneVirtualFile(t *testing.T) {
	result := runCommand(t, commandUnlink, []string{"gone"}, map[string]string{
		"gone": "remove me",
		"keep": "keep me",
	})
	if result.exitCode != 0 || result.stdout != "" || result.stderr != "" {
		t.Fatalf("result=%+v", result)
	}
	if exists(result.filesystem, "work/gone") || !exists(result.filesystem, "work/keep") {
		t.Fatalf("unexpected filesystem state after unlink")
	}
}

func TestUnlinkRequiresExactlyOneOperand(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "missing", want: "unlink: missing operand\n"},
		{name: "extra", args: []string{"one", "two"}, want: "unlink: extra operand 'two'\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := runCommand(t, commandUnlink, test.args, map[string]string{"one": "1", "two": "2"})
			if result.exitCode == 0 || result.stdout != "" || result.stderr != test.want {
				t.Fatalf("result=%+v", result)
			}
			if !exists(result.filesystem, "work/one") || !exists(result.filesystem, "work/two") {
				t.Fatal("operand error modified the filesystem")
			}
		})
	}
}

func TestUnlinkRejectsDirectories(t *testing.T) {
	result := runCommand(t, commandUnlink, []string{"."}, nil)
	if result.exitCode == 0 || result.stderr != "unlink: cannot unlink '.': Is a directory\n" {
		t.Fatalf("result=%+v", result)
	}
	if !exists(result.filesystem, "work") {
		t.Fatal("unlink removed a directory")
	}
}

func TestUnlinkReportsMissingAndUnsupportedRemove(t *testing.T) {
	result := runCommand(t, commandUnlink, []string{"missing"}, nil)
	if result.exitCode == 0 || !strings.Contains(result.stderr, "No such file or directory") {
		t.Fatalf("missing result=%+v", result)
	}

	filesystem := runCommand(t, commandUnlink, nil, map[string]string{"file": "data"}).filesystem
	result = runCommandWithStandardFS(t, commandUnlink, []string{"file"}, readOnlyTestFS{FS: filesystem})
	if result.exitCode == 0 || result.stderr == "" || !exists(filesystem, "work/file") {
		t.Fatalf("read-only result=%+v", result)
	}
}

func TestUnlinkDoubleDashAllowsDashOperand(t *testing.T) {
	result := runCommand(t, commandUnlink, []string{"--", "-file"}, map[string]string{"-file": "data"})
	if result.exitCode != 0 || exists(result.filesystem, "work/-file") {
		t.Fatalf("result=%+v", result)
	}
}
