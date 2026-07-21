package files

import (
	"strings"
	"testing"
)

func TestRegisteredFileCommandErrors(t *testing.T) {
	cases := []struct {
		name string
		run  commandFunc
		args []string
	}{
		{name: "ls", run: commandLS, args: []string{"--definitely-invalid"}},
		{name: "mkdir", run: commandMkdir, args: []string{"/missing/child"}},
		{name: "touch", run: commandTouch, args: []string{"/missing/file"}},
		{name: "rm", run: commandRM, args: []string{"missing"}},
		{name: "rmdir", run: commandRmdir, args: nil},
		{name: "cp", run: commandCPParity, args: []string{"only-source"}},
		{name: "mv", run: commandMV, args: []string{"only-source"}},
		{name: "ln", run: commandLNParity, args: []string{"only-source"}},
		{name: "chmod", run: commandChmod, args: []string{"644"}},
		{name: "stat", run: commandStat, args: nil},
		{name: "file", run: commandFile, args: nil},
		{name: "split", run: commandSplit, args: []string{"--bogus"}},
		{name: "readlink", run: commandReadlink, args: []string{"missing"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := runCommand(t, tc.run, tc.args, nil)
			if r.exitCode == 0 || r.stdout != "" || strings.TrimSpace(r.stderr) == "" {
				t.Fatalf("exit=%d stdout=%q stderr=%q", r.exitCode, r.stdout, r.stderr)
			}
		})
	}
}
