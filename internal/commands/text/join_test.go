package text

import "testing"

func TestJoin(t *testing.T) {
	assertCommand(t, commandJoin, []string{"-t", ",", "a", "b"}, "", "1,A,B\n", map[string]string{"a": "1,A\n", "b": "1,B\n"})
}
