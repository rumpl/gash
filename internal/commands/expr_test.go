package commands

import (
	"strings"
	"testing"
)

func TestExprArithmeticPrecedenceAndStatus(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantStdout string
		wantCode   int
	}{
		{name: "single operand", args: []string{"hello"}, wantStdout: "hello\n", wantCode: 0},
		{name: "zero operand status", args: []string{"0"}, wantStdout: "0\n", wantCode: 1},
		{name: "empty operand status", args: []string{""}, wantStdout: "\n", wantCode: 1},
		{name: "precedence", args: []string{"1", "+", "2", "*", "3"}, wantStdout: "7\n", wantCode: 0},
		{name: "parentheses", args: []string{"(", "1", "+", "2", ")", "*", "3"}, wantStdout: "9\n", wantCode: 0},
		{name: "division truncates", args: []string{"7", "/", "2"}, wantStdout: "3\n", wantCode: 0},
		{name: "negative division truncates toward zero", args: []string{"-7", "/", "2"}, wantStdout: "-3\n", wantCode: 0},
		{name: "modulo", args: []string{"7", "%", "4"}, wantStdout: "3\n", wantCode: 0},
		{name: "parseInt compatible prefix", args: []string{"12abc", "+", "3"}, wantStdout: "15\n", wantCode: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, stdout, stderr, _ := runCommand(t, commandExpr, tt.args, nil, nil)
			if code != tt.wantCode || string(stdout) != tt.wantStdout || stderr != "" {
				t.Fatalf("exit=%d stdout=%q stderr=%q, want exit=%d stdout=%q", code, stdout, stderr, tt.wantCode, tt.wantStdout)
			}
		})
	}
}

func TestExprComparisonBooleanAndStrings(t *testing.T) {
	tests := []struct {
		args       []string
		wantStdout string
		wantCode   int
	}{
		{[]string{"10", ">", "2"}, "1\n", 0},
		{[]string{"abc", "<", "b"}, "1\n", 0},
		{[]string{"05", "=", "5"}, "1\n", 0},
		{[]string{"a", "!=", "a"}, "0\n", 1},
		{[]string{"", "|", "fallback"}, "fallback\n", 0},
		{[]string{"left", "|", "fallback"}, "left\n", 0},
		{[]string{"left", "&", "right"}, "left\n", 0},
		{[]string{"left", "&", "0"}, "0\n", 1},
		{[]string{"0", "&", "right"}, "0\n", 1},
	}
	for _, tt := range tests {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			code, stdout, stderr, _ := runCommand(t, commandExpr, tt.args, nil, nil)
			if code != tt.wantCode || string(stdout) != tt.wantStdout || stderr != "" {
				t.Fatalf("exit=%d stdout=%q stderr=%q, want exit=%d stdout=%q", code, stdout, stderr, tt.wantCode, tt.wantStdout)
			}
		})
	}
}

func TestExprStringAndRegexOperations(t *testing.T) {
	tests := []struct {
		args       []string
		wantStdout string
		wantCode   int
	}{
		{[]string{"abcdef", ":", "abc"}, "3\n", 0},
		{[]string{"abcdef", ":", "a(b.)"}, "bc\n", 0},
		{[]string{"abcdef", ":", "z"}, "0\n", 1},
		{[]string{"match", "abcdef", "bcd"}, "3\n", 0},
		{[]string{"match", "abcdef", "b(c.)"}, "cd\n", 0},
		{[]string{"substr", "abcdef", "2", "3"}, "bcd\n", 0},
		{[]string{"substr", "abcdef", "4", "99"}, "def\n", 0},
		{[]string{"index", "abcdef", "dx"}, "4\n", 0},
		{[]string{"index", "abcdef", "xy"}, "0\n", 1},
		{[]string{"length", "hello"}, "5\n", 0},
		{[]string{"length", "😀"}, "2\n", 0},
		{[]string{"substr", "a😀b", "2", "2"}, "😀\n", 0},
	}
	for _, tt := range tests {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			code, stdout, stderr, _ := runCommand(t, commandExpr, tt.args, nil, nil)
			if code != tt.wantCode || string(stdout) != tt.wantStdout || stderr != "" {
				t.Fatalf("exit=%d stdout=%q stderr=%q, want exit=%d stdout=%q", code, stdout, stderr, tt.wantCode, tt.wantStdout)
			}
		})
	}
}

func TestExprErrors(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantError string
	}{
		{name: "missing operand", args: nil, wantError: "missing operand"},
		{name: "trailing input", args: []string{"1", "trailing"}, wantError: "syntax error"},
		{name: "extra close", args: []string{"1", ")"}, wantError: "syntax error"},
		{name: "trailing after expression", args: []string{"1", "+", "2", "trailing"}, wantError: "syntax error"},
		{name: "malformed right", args: []string{"1", "|", "2", "+"}, wantError: "syntax error"},
		{name: "missing close", args: []string{"1", "|", "("}, wantError: "syntax error"},
		{name: "non integer", args: []string{"abc", "+", "1"}, wantError: "non-integer argument"},
		{name: "division by zero", args: []string{"1", "/", "0"}, wantError: "division by zero"},
		{name: "bad substr pos", args: []string{"substr", "abc", "x", "1"}, wantError: "non-integer argument"},
		{name: "bad regex", args: []string{"abc", ":", "["}, wantError: "error parsing regexp"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, stdout, stderr, _ := runCommand(t, commandExpr, tt.args, nil, nil)
			if code != 2 || len(stdout) != 0 || !strings.Contains(stderr, "expr: "+tt.wantError) {
				t.Fatalf("exit=%d stdout=%q stderr=%q, want error containing %q", code, stdout, stderr, tt.wantError)
			}
		})
	}
}

func TestExprSecurityFailClosedParsing(t *testing.T) {
	for _, args := range [][]string{
		{"1", "trailing"},
		{"1", ")"},
		{"1", "+", "2", "trailing"},
		{"1", "|", "("},
	} {
		code, stdout, stderr, _ := runCommand(t, commandExpr, args, nil, nil)
		if code != 2 || len(stdout) != 0 || !strings.Contains(stderr, "syntax error") {
			t.Fatalf("args=%v exit=%d stdout=%q stderr=%q", args, code, stdout, stderr)
		}
	}

	code, stdout, stderr, _ := runCommand(t, commandExpr, []string{"1", "|", "2", "+"}, nil, nil)
	if code != 2 || len(stdout) != 0 || !strings.Contains(stderr, "syntax error") {
		t.Fatalf("truthy OR exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}

	code, stdout, stderr, _ = runCommand(t, commandExpr, []string{"index", "abcdef", "dx"}, nil, nil)
	if code != 0 || string(stdout) != "4\n" || stderr != "" {
		t.Fatalf("index exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}
