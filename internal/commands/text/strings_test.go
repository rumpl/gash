package text

import "testing"

func TestStrings(t *testing.T) {
	assertCommand(t, commandStrings, []string{"-n", "3"}, "abc\x00de", "abc\n", nil)
}
