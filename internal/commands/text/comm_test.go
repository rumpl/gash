package text

import "testing"

func TestComm(t *testing.T) {
	assertCommand(t, commandComm, []string{"a", "b"}, "", "a\n\t\tb\n\tc\n", map[string]string{"a": "a\nb\n", "b": "b\nc\n"})
}
