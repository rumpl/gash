package text

import "testing"

func TestPaste(t *testing.T) {
	assertCommand(t, commandPaste, []string{"-d", ":", "a", "b"}, "", "x:1\ny:2\n", map[string]string{"a": "x\ny\n", "b": "1\n2\n"})
}
