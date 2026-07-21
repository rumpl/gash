package text

import "testing"

func TestExpand(t *testing.T) {
	assertCommand(t, commandExpand, []string{"-t", "4"}, "a\tb\n", "a   b\n", nil)
}
