package text

import "testing"

func TestOd(t *testing.T) {
	assertCommand(t, commandOD, []string{"-x"}, "A", "0000000 41\n0000001\n", nil)
}
