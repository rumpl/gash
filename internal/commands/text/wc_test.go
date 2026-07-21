package text

import "testing"

func TestWc(t *testing.T) {
	assertCommand(t, commandWC, []string{"-l"}, "a\nb\n", "2\n", nil)
}

func TestWcCommonWorkflows(t *testing.T) {
	assertCommand(t, commandWC, []string{"-w"}, "hello gash\n世界\n", "3\n", nil)
	assertCommandBytes(t, commandWC, []string{"-c"}, []byte{'a', 0, 'b', 0xff, '\n'}, []byte("5\n"), nil)
	assertCommand(t, commandWC, []string{"-m"}, "é\n", "2\n", nil)
	assertCommand(t, commandWC, []string{"-l", "a.txt", "-", "b.txt"}, "stdin\n", "3\n", map[string]string{
		"a.txt": "a\n",
		"b.txt": "b\n",
	})
	assertCommand(t, commandWC, nil, "a\nb\n", "2 2 4\n", nil)

	code, stdout, stderr, _ := runTextCommandBytes(t, commandWC, []string{"--bogus"}, nil, nil)
	if code == 0 || len(stdout) != 0 || stderr == "" {
		t.Fatalf("invalid option exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	code, stdout, stderr, _ = runTextCommandBytes(t, commandWC, []string{"missing.txt"}, nil, nil)
	if code == 0 || len(stdout) != 0 || stderr == "" {
		t.Fatalf("missing file exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}
