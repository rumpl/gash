package text

import "testing"

func TestTr(t *testing.T) {
	assertCommand(t, commandTr, []string{"a-z", "A-Z"}, "abc", "ABC", nil)
}

func TestTrCommonWorkflows(t *testing.T) {
	assertCommand(t, commandTr, []string{"-d", "0-9"}, "a1b2\n", "ab\n", nil)
	assertCommand(t, commandTr, []string{"-s", "a"}, "baaaad\n", "bad\n", nil)
	assertCommand(t, commandTr, []string{"\\n", " "}, "a\nb\n", "a b ", nil)
	assertCommand(t, commandTr, []string{"åβ", "AB"}, "åβc", "ABc", nil)
	assertCommandBytes(t, commandTr, []string{"-d", "\\000"}, []byte{'a', 0, 'b', '\n'}, []byte("ab\n"), nil)

	code, stdout, stderr, _ := runTextCommandBytes(t, commandTr, nil, nil, nil)
	if code == 0 || len(stdout) != 0 || stderr == "" {
		t.Fatalf("missing operand exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	code, stdout, stderr, _ = runTextCommandBytes(t, commandTr, []string{"--bogus", "a"}, nil, nil)
	if code == 0 || len(stdout) != 0 || stderr == "" {
		t.Fatalf("invalid option exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}
