package text

import "testing"

func TestUnexpand(t *testing.T) {
	assertCommand(t, commandUnexpand, []string{"-a", "-t", "4"}, "    x\n", "\tx\n", nil)
}
