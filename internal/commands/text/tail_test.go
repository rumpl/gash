package text

import "testing"

func TestTail(t *testing.T) {
	assertCommand(t, commandTail, []string{"-n", "1"}, "a\nb\n", "b\n", nil)
}
