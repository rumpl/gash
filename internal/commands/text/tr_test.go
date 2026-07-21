package text

import "testing"

func TestTr(t *testing.T) {
	assertCommand(t, commandTr, []string{"a-z", "A-Z"}, "abc", "ABC", nil)
}
