package text

import "testing"

func TestOd(t *testing.T) {
	assertCommand(t, commandOD, []string{"-x"}, "A", "0000000 41\n0000001\n", nil)
	assertCommand(t, commandOD, []string{"-tx1"}, "x", "0000000 78\n0000001\n", nil)
	assertCommand(t, commandOD, []string{"-An", "-tx1"}, "x", " 78\n", nil)
}

func TestOdRejectsUnsupportedOptionsAndTypes(t *testing.T) {
	code, stdout, stderr, _ := runTextCommandBytes(t, commandOD, []string{"-z"}, []byte("x"), nil)
	if code == 0 || len(stdout) != 0 || stderr == "" {
		t.Fatalf("option exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	code, stdout, stderr, _ = runTextCommandBytes(t, commandOD, []string{"-tf8"}, []byte("x"), nil)
	if code == 0 || len(stdout) != 0 || stderr == "" {
		t.Fatalf("type exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}
