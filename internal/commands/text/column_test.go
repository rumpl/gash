package text

import "testing"

func TestColumn(t *testing.T) {
	assertCommand(t, commandColumn, []string{"-t", "-s", ":"}, "x:y\nlong:z\n", "x     y\nlong  z\n", nil)
}
