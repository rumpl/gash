package text

import "testing"

func TestTee(t *testing.T) {
	assertCommand(t, commandTee, nil, "copied\n", "copied\n", nil)
}
