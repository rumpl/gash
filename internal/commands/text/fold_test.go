package text

import "testing"

func TestFold(t *testing.T) {
	assertCommand(t, commandFold, []string{"-w", "2"}, "abcd\n", "ab\ncd\n", nil)
}
