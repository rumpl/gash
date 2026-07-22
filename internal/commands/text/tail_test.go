package text

import "testing"

func TestTail(t *testing.T) {
	assertCommand(t, commandTail, []string{"-n", "1"}, "a\nb\n", "b\n", nil)
	assertCommand(t, commandTail, []string{"-1"}, "a\nb\n", "b\n", nil)
}

func TestTailCommonWorkflows(t *testing.T) {
	assertCommand(t, commandTail, []string{"-n", "2", "a.txt", "-", "b.txt"}, "stdin1\nstdin2\nstdin3\n", "a2\na3\nstdin2\nstdin3\nb1\nb2\n", map[string]string{
		"a.txt": "a1\na2\na3\n",
		"b.txt": "b1\nb2\n",
	})
	assertCommand(t, commandTail, []string{"-n", "+2"}, "a\nb\nc\n", "b\nc\n", nil)
	assertCommandBytes(t, commandTail, []string{"-c", "3"}, []byte{'a', 0, 'b', 0xff, 'c'}, []byte{'b', 0xff, 'c'}, nil)

	code, stdout, stderr, _ := runTextCommandBytes(t, commandTail, []string{"--bogus"}, nil, nil)
	if code == 0 || len(stdout) != 0 || stderr == "" {
		t.Fatalf("invalid option exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	code, stdout, stderr, _ = runTextCommandBytes(t, commandTail, []string{"missing.txt"}, nil, nil)
	if code == 0 || len(stdout) != 0 || stderr == "" {
		t.Fatalf("missing file exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}
