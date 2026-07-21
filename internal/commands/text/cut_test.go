package text

import "testing"

func TestCut(t *testing.T) {
	assertCommand(t, commandCut, []string{"-d", ",", "-f", "2"}, "1,Ada\n", "Ada\n", nil)
}

func TestCutCommonWorkflows(t *testing.T) {
	assertCommand(t, commandCut, []string{"-d,", "-f1,3", "a.txt", "-"}, "s1,s2,s3\n", "a1,a3\ns1,s3\n", map[string]string{"a.txt": "a1,a2,a3\n"})
	assertCommand(t, commandCut, []string{"-d", ",", "-s", "-f", "2"}, "no-delimiter\n1,2\n", "2\n", nil)
	assertCommand(t, commandCut, []string{"-c", "2-3"}, "åβc\n", "βc\n", nil)
	assertCommandBytes(t, commandCut, []string{"-b", "2-4"}, []byte{'a', 0, 'b', 0xff, '\n'}, []byte{0, 'b', 0xff, '\n'}, nil)

	code, stdout, stderr, _ := runTextCommandBytes(t, commandCut, []string{"-z"}, nil, nil)
	if code == 0 || len(stdout) != 0 || stderr == "" {
		t.Fatalf("invalid/missing option exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	code, stdout, stderr, _ = runTextCommandBytes(t, commandCut, []string{"-f", "1", "missing.txt"}, nil, nil)
	if code == 0 || len(stdout) != 0 || stderr == "" {
		t.Fatalf("missing file exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}
