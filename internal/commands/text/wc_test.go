package text

import "testing"

func TestWc(t *testing.T) {
	assertCommand(t, commandWC, []string{"-l"}, "a\nb\n", "2\n", nil)
}
