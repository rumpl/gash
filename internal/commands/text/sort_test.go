package text

import "testing"

func TestSort(t *testing.T) {
	assertCommand(t, commandSort, nil, "b\na\n", "a\nb\n", nil)
}
