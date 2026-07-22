package text

import "testing"

func TestUniq(t *testing.T) {
	assertCommand(t, commandUniq, nil, "a\na\nb\n", "a\nb\n", nil)
}

func TestUniqCount(t *testing.T) {
	assertCommand(t, commandUniq, []string{"-c"}, "a\na\nb\n", "   2 a\n   1 b\n", nil)
}

func TestUniqFiltersAndIgnoresCase(t *testing.T) {
	assertCommand(t, commandUniq, []string{"-di"}, "A\na\nb\nc\nc\n", "A\nc\n", nil)
	assertCommand(t, commandUniq, []string{"--unique"}, "a\na\nb\nc\nc\n", "b\n", nil)
}

func TestUniqRejectsUnknownOption(t *testing.T) {
	code, stdout, stderr, _ := runTextCommandBytes(t, commandUniq, []string{"--bogus"}, nil, nil)
	if code == 0 || len(stdout) != 0 || stderr == "" {
		t.Fatalf("exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}
