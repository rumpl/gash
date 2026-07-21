package gash

import (
	"context"
	"strings"
	"testing"
)

func TestCutPasteCommAndJoin(t *testing.T) {
	b := newTestBash(t)
	script := `printf '1,alice\n2,bob\n' > people; cut -d, -f2 people; printf 'a\nb\n' > left; printf '1\n2\n' > right; paste -d: left right; printf 'a\nb\n' > one; printf 'b\nc\n' > two; comm one two; join -t, people people`
	result := b.Exec(context.Background(), script, ExecOptions{})
	if result.ExitCode != 0 {
		t.Fatalf("%+v", result)
	}
	for _, want := range []string{"alice\nbob\n", "a:1\nb:2\n", "a\n\t\tb\n\tc\n", "1,alice,alice"} {
		if !strings.Contains(result.Stdout, want) {
			t.Fatalf("missing %q in %q", want, result.Stdout)
		}
	}
}

func TestCharacterTransformCommands(t *testing.T) {
	b := newTestBash(t)
	result := b.Exec(context.Background(), `printf 'abc\ndef\n' | rev; printf 'one\ntwo\n' | tac; printf 'abc  123' | tr -s ' ' | tr 'a-z' 'A-Z'; printf 'abcdef' | fold -w 3`, ExecOptions{})
	if result.ExitCode != 0 {
		t.Fatalf("%+v", result)
	}
	for _, want := range []string{"cba\nfed\n", "two\none\n", "ABC 123", "abc\ndef\n"} {
		if !strings.Contains(result.Stdout, want) {
			t.Fatalf("missing %q in %q", want, result.Stdout)
		}
	}
}

func TestFormattingAndBinaryInspectionCommands(t *testing.T) {
	b := newTestBash(t)
	result := b.Exec(context.Background(), `printf 'a\tb\n' | expand -t 4; printf 'x:y\nlong:z\n' | column -t -s :; printf 'hello\000world' | strings -n 5; printf 'AB' | od -x; printf 'first\n\nthird\n' | nl -ba`, ExecOptions{})
	if result.ExitCode != 0 {
		t.Fatalf("%+v", result)
	}
	for _, want := range []string{"a   b", "x     y\nlong  z", "hello\nworld", "0000000 41 42", "     1\tfirst", "     2\t"} {
		if !strings.Contains(result.Stdout, want) {
			t.Fatalf("missing %q in %q", want, result.Stdout)
		}
	}
}
