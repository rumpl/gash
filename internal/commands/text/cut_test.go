package text

import "testing"

func TestCut(t *testing.T) {
	assertCommand(t, commandCut, []string{"-d", ",", "-f", "2"}, "1,Ada\n", "Ada\n", nil)
}
