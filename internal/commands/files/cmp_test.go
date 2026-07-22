package files

import "testing"

func TestCmpIdenticalAndDifferentFiles(t *testing.T) {
	result := runCommand(t, commandCmp, []string{"left", "same"}, map[string]string{
		"left": "one\ntwo\n",
		"same": "one\ntwo\n",
	})
	if result.exitCode != 0 || result.stdout != "" || result.stderr != "" {
		t.Fatalf("identical result=%+v", result)
	}

	result = runCommand(t, commandCmp, []string{"left", "right"}, map[string]string{
		"left":  "one\ntwo\n",
		"right": "one\ntXo\n",
	})
	if result.exitCode != 1 || result.stdout != "left right differ: byte 6, line 2\n" || result.stderr != "" {
		t.Fatalf("different result=%+v", result)
	}
}

func TestCmpSilentVerboseLimitAndSkip(t *testing.T) {
	files := map[string]string{"left": "abcX", "right": "abcY"}
	result := runCommand(t, commandCmp, []string{"-s", "left", "right"}, files)
	if result.exitCode != 1 || result.stdout != "" || result.stderr != "" {
		t.Fatalf("silent result=%+v", result)
	}
	result = runCommand(t, commandCmp, []string{"-l", "left", "right"}, files)
	if result.exitCode != 1 || result.stdout != "4 130 131\n" || result.stderr != "" {
		t.Fatalf("verbose result=%+v", result)
	}
	result = runCommand(t, commandCmp, []string{"-n", "3", "left", "right"}, files)
	if result.exitCode != 0 {
		t.Fatalf("limit result=%+v", result)
	}
	result = runCommand(t, commandCmp, []string{"left", "right", "4", "4"}, files)
	if result.exitCode != 0 {
		t.Fatalf("skip result=%+v", result)
	}
}

func TestCmpStandardInputAndEOF(t *testing.T) {
	result := runCommand(t, commandCmp, []string{"left"}, map[string]string{"left": ""})
	if result.exitCode != 0 || result.stdout != "" || result.stderr != "" {
		t.Fatalf("stdin result=%+v", result)
	}
	result = runCommand(t, commandCmp, []string{"short", "long"}, map[string]string{"short": "a", "long": "ab"})
	if result.exitCode != 1 || result.stdout != "" || result.stderr != "cmp: EOF on short after byte 1, line 1\n" {
		t.Fatalf("EOF result=%+v", result)
	}
}
