package text

import "testing"

func TestTac(t *testing.T) {
	assertCommand(t, commandTac, nil, "one\ntwo\n", "two\none\n", nil)
}
