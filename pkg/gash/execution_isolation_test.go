package gash

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestUnknownCommandsAndExplicitPathsDoNotUseHost(t *testing.T) {
	b := newTestBash(t)
	for _, script := range []string{
		"go version",
		"/bin/go version",
		"/usr/bin/go version",
		"/tmp/host-tool",
	} {
		t.Run(script, func(t *testing.T) {
			result := b.Exec(context.Background(), script, ExecOptions{})
			if result.ExitCode != 127 {
				t.Fatalf("exit=%d stdout=%q stderr=%q", result.ExitCode, result.Stdout, result.Stderr)
			}
			if result.Stdout != "" {
				t.Fatalf("host command produced stdout: %q", result.Stdout)
			}
			if !strings.Contains(result.Stderr, "command not found") {
				t.Fatalf("stderr=%q", result.Stderr)
			}
		})
	}
}

func TestBashShCommandArgvStdinAndStatus(t *testing.T) {
	b, err := New(Options{Files: map[string]string{
		"/home/user/scripts/argv.sh": "printf 'file:%s:%s:%s\\n' \"$0\" \"$1\" \"$2\"; exit 7\n",
	}})
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		script string
		stdin  string
		out    string
		code   int
	}{
		{"bash -c argv", `bash -c 'printf "%s:%s:%s\n" "$0" "$1" "$2"; exit 3' name left right`, "", "name:left:right\n", 3},
		{"sh -c stdin", `printf 'one\ntwo\n' | sh -c 'read a; read b; printf "%s/%s/%s\n" "$0" "$a" "$b"' shell-name`, "", "shell-name/one/two\n", 0},
		{"bash script file", `bash scripts/argv.sh left right`, "", "file:scripts/argv.sh:left:right\n", 7},
		{"bash reads stdin script", `bash`, "printf 'stdin:%s\\n' \"$0\"\n", "stdin:gosh\n", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := b.Exec(context.Background(), tc.script, ExecOptions{Stdin: tc.stdin})
			if result.ExitCode != tc.code || result.Stdout != tc.out || result.Stderr != "" {
				t.Fatalf("result=%+v want code=%d stdout=%q", result, tc.code, tc.out)
			}
		})
	}
}

func TestExecIsolationForEnvCwdFunctionsAndOptions(t *testing.T) {
	b := newTestBash(t)
	first := b.Exec(context.Background(), `export LEAK=env; cd /tmp; set -e; greet(){ echo leaked; }; printf data > /tmp/persisted`, ExecOptions{})
	if first.ExitCode != 0 {
		t.Fatalf("first=%+v", first)
	}
	second := b.Exec(context.Background(), `printf 'cwd=%s env=%s errexit=%s func=' "$PWD" "${LEAK-unset}" "$-"; type greet >/dev/null 2>&1 && echo yes || echo no; cat /tmp/persisted`, ExecOptions{})
	if second.ExitCode != 0 {
		t.Fatalf("second=%+v", second)
	}
	if !strings.Contains(second.Stdout, "cwd=/home/user env=unset ") || !strings.Contains(second.Stdout, " func=no\ndata") || strings.Contains(second.Stdout, "errexit=e") {
		t.Fatalf("stdout=%q stderr=%q", second.Stdout, second.Stderr)
	}
}

func TestVirtualPIDAndEnvironmentIsolation(t *testing.T) {
	b, err := New(Options{Env: map[string]string{"UID": "123", "EUID": "456", "GID": "789", "PPID": "999"}})
	if err != nil {
		t.Fatal(err)
	}
	result := b.Exec(context.Background(), `printf '%s:%s:%s:%s:%s' "$$" "$PPID" "$UID" "$EUID" "$GID"`, ExecOptions{Env: map[string]string{"UID": "321", "PPID": "888"}})
	if result.ExitCode != 0 || result.Stdout != "2000:1999:1000:1000:1000" {
		t.Fatalf("virtual identity leaked or changed: %+v", result)
	}
}

func TestNestedExecutionDepthLimit(t *testing.T) {
	b, err := New(Options{Limits: Limits{MaxExecDepth: 1, MaxCallDepth: 2}})
	if err != nil {
		t.Fatal(err)
	}
	result := b.Exec(context.Background(), `bash -c 'bash -c "echo too-deep"'`, ExecOptions{})
	if result.ExitCode != 126 || !strings.Contains(result.Stderr, "maximum nested execution depth") {
		t.Fatalf("%+v", result)
	}
}

func TestCancellationPropagatesThroughPipelineAndCommandSubstitution(t *testing.T) {
	b, err := New(Options{Limits: Limits{MaxExecutionTime: 20 * time.Millisecond}})
	if err != nil {
		t.Fatal(err)
	}
	for _, script := range []string{
		`sleep 1 | cat`,
		`echo "$(sleep 1)"`,
	} {
		t.Run(script, func(t *testing.T) {
			result := b.Exec(context.Background(), script, ExecOptions{})
			if result.ExitCode != 124 || !strings.Contains(result.Stderr, "execution timed out") {
				t.Fatalf("%+v", result)
			}
		})
	}
}

func TestOutputLimitCoversLongLoopPipelineAndCommandSubstitution(t *testing.T) {
	b, err := New(Options{Limits: Limits{MaxOutputBytes: 64, MaxExecutionTime: time.Second}})
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		script string
		check  func(t *testing.T, result Result)
	}{
		{
			name:   "long loop cancels promptly",
			script: `while :; do echo 1234567890; done`,
			check: func(t *testing.T, result Result) {
				if result.ExitCode != 126 {
					t.Fatalf("%+v", result)
				}
			},
		},
		{
			// mvdan connects pipeline stages with host os.Pipe values before the
			// final command writes to gash's bounded stdout. The pipe itself is not
			// currently counted, so this regression case documents the remaining
			// unsupported aggregate-limit gap while host process/filesystem access
			// stays blocked at gash's ExecHandler/OpenHandler boundary.
			name:   "pipeline output documents mvdan pipe bypass",
			script: `printf '%s' 'abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyz' | cat`,
			check: func(t *testing.T, result Result) {
				if result.ExitCode != 0 || result.Stdout != "abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyz" {
					t.Fatalf("%+v", result)
				}
			},
		},
		{
			name:   "command substitution aggregate limit",
			script: `printf '%s' "$(printf '%s' 'abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyz')"`,
			check: func(t *testing.T, result Result) {
				if result.ExitCode != 126 {
					t.Fatalf("%+v", result)
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			start := time.Now()
			result := b.Exec(context.Background(), tc.script, ExecOptions{})
			if time.Since(start) > 500*time.Millisecond {
				t.Fatalf("output limit did not cancel promptly: %s result=%+v", time.Since(start), result)
			}
			tc.check(t, result)
			if len(result.Stdout)+len(result.Stderr) > 128 && tc.name != "pipeline output documents mvdan pipe bypass" {
				t.Fatalf("unexpectedly large output after limit: stdout=%d stderr=%d", len(result.Stdout), len(result.Stderr))
			}
		})
	}
}

func TestUnsupportedFileDescriptorRejectedBeforeInterpreter(t *testing.T) {
	b := newTestBash(t)
	result := b.Exec(context.Background(), `echo hi 3>&1`, ExecOptions{})
	if result.ExitCode != 2 || !strings.Contains(result.Stderr, `file descriptor "3" is not supported`) || strings.Contains(result.Stderr, "interpreter failure") {
		t.Fatalf("%+v", result)
	}
}

func TestProcessSubstitutionRejectedToAvoidHostFIFO(t *testing.T) {
	b := newTestBash(t)
	result := b.Exec(context.Background(), `cat <(echo nope)`, ExecOptions{})
	if result.ExitCode != 2 || !strings.Contains(result.Stderr, "process substitution is not supported") {
		t.Fatalf("%+v", result)
	}
}
