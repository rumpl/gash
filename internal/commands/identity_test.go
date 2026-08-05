package commands

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/rumpl/gash/internal/command"
)

func TestIDOutput(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "default", want: "uid=1000(user) gid=1000(user) groups=1000(user)\n"},
		{name: "uid", args: []string{"-u"}, want: "1000\n"},
		{name: "gid", args: []string{"-g"}, want: "1000\n"},
		{name: "groups", args: []string{"-G"}, want: "1000\n"},
		{name: "user name", args: []string{"-un"}, want: "user\n"},
		{name: "name user", args: []string{"-nu"}, want: "user\n"},
		{name: "group name", args: []string{"-gn"}, want: "user\n"},
		{name: "name group", args: []string{"-ng"}, want: "user\n"},
		{name: "groups names", args: []string{"-Gn"}, want: "user\n"},
		{name: "name groups", args: []string{"-nG"}, want: "user\n"},
		{name: "long uid", args: []string{"--user"}, want: "1000\n"},
		{name: "long group name", args: []string{"--group", "--name"}, want: "user\n"},
		{name: "long groups name", args: []string{"--name", "--groups"}, want: "user\n"},
		{name: "option terminator", args: []string{"-u", "--"}, want: "1000\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, stdout, stderr := runID(tt.args, nil)
			if code != 0 || stdout != tt.want || stderr != "" {
				t.Fatalf("exit=%d stdout=%q stderr=%q, want stdout=%q", code, stdout, stderr, tt.want)
			}
		})
	}
}

func TestIDRejectsUnsupportedArguments(t *testing.T) {
	tests := []struct {
		name string
		args []string
		text string
	}{
		{name: "bare name", args: []string{"-n"}, text: "requires exactly one"},
		{name: "multiple selectors", args: []string{"-ug"}, text: "cannot print more than one"},
		{name: "separate selectors", args: []string{"--user", "--groups"}, text: "cannot print more than one"},
		{name: "repeated selector", args: []string{"-u", "-u"}, text: "cannot print more than one"},
		{name: "short flag", args: []string{"-r"}, text: "invalid option"},
		{name: "long flag", args: []string{"--real"}, text: "unrecognized option"},
		{name: "operand", args: []string{"user"}, text: "operands are unsupported"},
		{name: "operand after terminator", args: []string{"--", "user"}, text: "operands are unsupported"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, stdout, stderr := runID(tt.args, nil)
			if code == 0 || stdout != "" || !strings.Contains(stderr, tt.text) {
				t.Fatalf("exit=%d stdout=%q stderr=%q", code, stdout, stderr)
			}
		})
	}
}

func TestIDIgnoresEnvironmentIdentity(t *testing.T) {
	env := map[string]string{
		"USER": "host-user",
		"UID":  "42",
		"EUID": "43",
		"GID":  "44",
	}
	code, stdout, stderr := runID(nil, env)
	if code != 0 || stdout != "uid=1000(user) gid=1000(user) groups=1000(user)\n" || stderr != "" {
		t.Fatalf("exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

func runID(args []string, env map[string]string) (int, string, string) {
	var stdout, stderr bytes.Buffer
	ctx := &command.Context{Env: env, Stdout: &stdout, Stderr: &stderr}
	code := commandID(context.Background(), args, ctx)
	return code, stdout.String(), stderr.String()
}
