package commands

import (
	"runtime"
	"strings"
	"testing"
)

func TestUnameDeterministicFields(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "default", want: "Gash\n"},
		{name: "kernel short", args: []string{"-s"}, want: "Gash\n"},
		{name: "nodename short", args: []string{"-n"}, want: "localhost\n"},
		{name: "release short", args: []string{"-r"}, want: "1.0.0\n"},
		{name: "version short", args: []string{"-v"}, want: "#1 gash\n"},
		{name: "machine short", args: []string{"-m"}, want: "virtual\n"},
		{name: "all short", args: []string{"-a"}, want: "Gash localhost 1.0.0 #1 gash virtual\n"},
		{name: "combined", args: []string{"-ms"}, want: "Gash virtual\n"},
		{name: "separate reordered", args: []string{"-m", "-n", "-s"}, want: "Gash localhost virtual\n"},
		{name: "all long", args: []string{"--all"}, want: "Gash localhost 1.0.0 #1 gash virtual\n"},
		{name: "kernel long", args: []string{"--kernel-name"}, want: "Gash\n"},
		{name: "nodename long", args: []string{"--nodename"}, want: "localhost\n"},
		{name: "release long", args: []string{"--kernel-release"}, want: "1.0.0\n"},
		{name: "version long", args: []string{"--kernel-version"}, want: "#1 gash\n"},
		{name: "machine long", args: []string{"--machine"}, want: "virtual\n"},
		{name: "option terminator", args: []string{"--"}, want: "Gash\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			code, stdout, stderr, _ := runCommand(t, commandUname, test.args, nil, nil)
			if code != 0 || string(stdout) != test.want || stderr != "" {
				t.Fatalf("exit=%d stdout=%q stderr=%q", code, stdout, stderr)
			}
			for _, hostValue := range []string{runtime.GOOS, runtime.GOARCH} {
				if hostValue != "" && hostValue != "virtual" && strings.Contains(string(stdout), hostValue) {
					t.Fatalf("stdout %q leaks host value %q", stdout, hostValue)
				}
			}
		})
	}
}

func TestUnameRejectsInvalidArguments(t *testing.T) {
	for _, args := range [][]string{{"-p"}, {"--processor"}, {"-sx"}, {"operand"}, {"--", "operand"}, {"-"}} {
		code, stdout, stderr, _ := runCommand(t, commandUname, args, nil, nil)
		if code == 0 || len(stdout) != 0 || stderr == "" {
			t.Fatalf("args=%q exit=%d stdout=%q stderr=%q", args, code, stdout, stderr)
		}
	}
}
