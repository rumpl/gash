package text

import "testing"

func TestGrep(t *testing.T) {
	assertCommand(t, commandGrep, []string{"needle"}, "hay\nneedle\n", "needle\n", nil)
}
