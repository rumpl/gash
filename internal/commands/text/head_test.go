package text

import "testing"

func TestHead(t *testing.T) {
	assertCommand(t, commandHead, []string{"-n", "1"}, "a\nb\n", "a\n", nil)
}
