package files

import (
	"strings"
	"testing"
)

func TestLs(t *testing.T) {
	result := runCommand(t, commandLS, []string{"."}, map[string]string{"a": "x"})
	if result.exitCode != 0 || result.stdout != "a\n" {
		t.Fatalf("exit=%d stdout=%q stderr=%q", result.exitCode, result.stdout, result.stderr)
	}
}

func TestLsDirectoryLongModesAndDotEntries(t *testing.T) {
	result := runCommand(t, commandLS, []string{"-d", "."}, map[string]string{"README.md": "docs"})
	if result.exitCode != 0 || result.stdout != ".\n" || result.stderr != "" {
		t.Fatalf("ls -d: %+v", result)
	}

	result = runCommand(t, commandLS, []string{"-l", "README.md"}, map[string]string{"README.md": "docs"})
	if result.exitCode != 0 || result.stdout != "-rw-r--r--        4 README.md\n" || result.stderr != "" {
		t.Fatalf("ls -l: %+v", result)
	}

	result = runCommand(t, commandLS, []string{"-a"}, map[string]string{".hidden": "h", "visible": "v"})
	if result.exitCode != 0 || result.stdout != ".\n..\n.hidden\nvisible\n" || result.stderr != "" {
		t.Fatalf("ls -a: %+v", result)
	}

	result = runCommand(t, commandLS, []string{"-A"}, map[string]string{".hidden": "h", "visible": "v"})
	if result.exitCode != 0 || result.stdout != ".hidden\nvisible\n" || result.stderr != "" {
		t.Fatalf("ls -A: %+v", result)
	}
}

func TestLsHelpMatchesUpstreamShape(t *testing.T) {
	result := runCommand(t, commandLS, []string{"--help"}, nil)
	if result.exitCode != 0 {
		t.Fatalf("exit=%d stderr=%q", result.exitCode, result.stderr)
	}
	for _, expected := range []string{
		"ls - list directory contents",
		"Usage: ls [OPTION]... [FILE]...",
		"Options:",
		"-a, --all",
		"--help",
	} {
		if !strings.Contains(result.stdout, expected) {
			t.Fatalf("help missing %q:\n%s", expected, result.stdout)
		}
	}
}
