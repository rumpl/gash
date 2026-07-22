package text

import "testing"

func TestPaste(t *testing.T) {
	assertCommand(t, commandPaste, []string{"-d", ":", "a", "b"}, "", "x:1\ny:2\n", map[string]string{"a": "x\ny\n", "b": "1\n2\n"})
}

func TestPasteCombinedOptionsWithExplicitStdin(t *testing.T) {
	assertCommand(t, commandPaste, []string{"-sd,", "-"}, "a\nb\n", "a,b\n", nil)
}

func TestPasteRepeatedStdinReadsSequentially(t *testing.T) {
	assertCommand(t, commandPaste, []string{"-", "-"}, "a\nb\nc\nd\n", "a\tb\nc\td\n", nil)
}
