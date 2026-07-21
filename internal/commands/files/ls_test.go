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
