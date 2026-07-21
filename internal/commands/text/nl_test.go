package text

import "testing"

func TestNl(t *testing.T) {
	assertCommand(t, commandNL, nil, "line\n", "     1\tline\n", nil)
}
