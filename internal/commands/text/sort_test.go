package text

import "testing"

func TestSort(t *testing.T) {
	assertCommand(t, commandSort, nil, "b\na\n", "a\nb\n", nil)
}

func TestSortCommonWorkflows(t *testing.T) {
	assertCommand(t, commandSort, []string{"-r", "a.txt", "-", "b.txt"}, "c\n", "c\nb\na\n", map[string]string{
		"a.txt": "b\n",
		"b.txt": "a\n",
	})
	assertCommand(t, commandSort, []string{"-n"}, "10\n2\n1\n", "1\n2\n10\n", nil)
	assertCommand(t, commandSort, []string{"-f", "-u"}, "b\nA\na\n", "A\nb\n", nil)
	assertCommand(t, commandSort, []string{"-o", "out.txt"}, "β\na\n", "", nil)

	code, stdout, stderr, fsys := runTextCommandBytes(t, commandSort, []string{"-o", "out.txt"}, []byte("β\na\n"), nil)
	if code != 0 || len(stdout) != 0 || stderr != "" {
		t.Fatalf("sort -o exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	data, err := fsys.ReadFile("work/out.txt")
	if err != nil || string(data) != "a\nβ\n" {
		t.Fatalf("sort -o data=%q err=%v", data, err)
	}

	code, stdout, stderr, _ = runTextCommandBytes(t, commandSort, []string{"--bogus"}, nil, nil)
	if code == 0 || len(stdout) != 0 || stderr == "" {
		t.Fatalf("invalid option exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	code, stdout, stderr, _ = runTextCommandBytes(t, commandSort, []string{"missing.txt"}, nil, nil)
	if code == 0 || len(stdout) != 0 || stderr == "" {
		t.Fatalf("missing file exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}
