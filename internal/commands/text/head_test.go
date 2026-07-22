package text

import "testing"

func TestHead(t *testing.T) {
	assertCommand(t, commandHead, []string{"-n", "1"}, "a\nb\n", "a\n", nil)
}

func TestHeadCommonWorkflows(t *testing.T) {
	assertCommand(t, commandHead, nil, "1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n11\n", "1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n", nil)
	assertCommand(t, commandHead, []string{"-n2", "a.txt", "-", "b.txt"}, "stdin1\nstdin2\nstdin3\n", "a1\na2\nstdin1\nstdin2\nb1\nb2\n", map[string]string{
		"a.txt": "a1\na2\na3\n",
		"b.txt": "b1\nb2\nb3\n",
	})
	assertCommandBytes(t, commandHead, []string{"-c", "4"}, []byte{'a', 0, 'b', 0xff, 'c'}, []byte{'a', 0, 'b', 0xff}, nil)

	code, stdout, stderr, _ := runTextCommandBytes(t, commandHead, []string{"--bogus"}, nil, nil)
	if code == 0 || len(stdout) != 0 || stderr == "" {
		t.Fatalf("invalid option exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	code, stdout, stderr, _ = runTextCommandBytes(t, commandHead, []string{"missing.txt"}, nil, nil)
	if code == 0 || len(stdout) != 0 || stderr == "" {
		t.Fatalf("missing file exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}
