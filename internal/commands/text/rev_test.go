package text

import "testing"

func TestRev(t *testing.T) {
	assertCommand(t, commandRev, nil, "abc\n", "cba\n", nil)
}
