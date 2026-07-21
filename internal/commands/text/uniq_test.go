package text

import "testing"

func TestUniq(t *testing.T) {
	assertCommand(t, commandUniq, nil, "a\na\nb\n", "a\nb\n", nil)
}
