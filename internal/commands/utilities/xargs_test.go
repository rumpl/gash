// Tests in this file port behavior from just-bash xargs.test.ts,
// xargs.command-name-quoting.test.ts, and xargs.utf8-stdin.test.ts at the
// upstream commit pinned in docs/status/feature-support.json.
package utilities

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rumpl/gash/internal/command"
)

func TestXargsBatchesArguments(t *testing.T) {
	result := runXargsTest(t, []string{"-n", "2", "echo"}, "a b c d e", echoRunner)
	if result.exitCode != 0 || result.stdout != "a b\nc d\ne\n" || result.stderr != "" {
		t.Fatalf("result=%+v", result)
	}
}

func TestXargsAcceptsAttachedOptionArguments(t *testing.T) {
	result := runXargsTest(t, []string{"-n1", "echo"}, "a b c", echoRunner)
	if result.exitCode != 0 || result.stdout != "a\nb\nc\n" || result.stderr != "" {
		t.Fatalf("result=%+v", result)
	}
	result = runXargsTest(t, []string{"-d:", "-I{}", "echo", "item={}"}, "a:b", echoRunner)
	if result.exitCode != 0 || result.stdout != "item=a\nitem=b\n" || result.stderr != "" {
		t.Fatalf("result=%+v", result)
	}
}

func TestXargsReplacementAndDelimiter(t *testing.T) {
	result := runXargsTest(t, []string{"-d", ":", "-I", "{}", "echo", "item: {}"}, "x:y:z\n", echoRunner)
	if result.exitCode != 0 || result.stdout != "item: x\nitem: y\nitem: z\n" {
		t.Fatalf("result=%+v", result)
	}
}

func TestXargsNullSeparatorPreservesUTF8AndSpaces(t *testing.T) {
	result := runXargsTest(t, []string{"-0", "-n", "1", "echo"}, "한 글\x00café\x00漢字\x00", echoRunner)
	if result.exitCode != 0 || result.stdout != "한 글\ncafé\n漢字\n" {
		t.Fatalf("result=%+v", result)
	}
}

func TestXargsVerboseQuotesCommands(t *testing.T) {
	result := runXargsTest(t, []string{"-t", "echo"}, "hello world", echoRunner)
	if result.exitCode != 0 || result.stdout != "hello world\n" || result.stderr != "echo hello world\n" {
		t.Fatalf("result=%+v", result)
	}
}

func TestXargsDoesNotRunForEmptyInput(t *testing.T) {
	var calls atomic.Int32
	runner := func(_ context.Context, _ []string, _ *command.Context) int {
		calls.Add(1)
		return 0
	}
	result := runXargsTest(t, []string{"echo"}, " \n\t", runner)
	if result.exitCode != 0 || result.stdout != "" || result.stderr != "" {
		t.Fatalf("result=%+v", result)
	}
	if calls.Load() != 0 {
		t.Fatalf("command ran %d times", calls.Load())
	}
}

func TestXargsRejectsInvalidNumbers(t *testing.T) {
	result := runXargsTest(t, []string{"-n", "0", "echo"}, "a", echoRunner)
	if result.exitCode != 1 || result.stderr != "xargs: invalid number for -n: '0'\n" {
		t.Fatalf("result=%+v", result)
	}
}

func TestXargsRunsParallelBatchesAndPreservesOutputOrder(t *testing.T) {
	var active atomic.Int32
	var maximum atomic.Int32
	runner := func(_ context.Context, argv []string, commandCtx *command.Context) int {
		current := active.Add(1)
		for {
			observed := maximum.Load()
			if current <= observed || maximum.CompareAndSwap(observed, current) {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
		fmt.Fprintln(commandCtx.Stdout, argv[1])
		active.Add(-1)
		return 0
	}
	result := runXargsTest(t, []string{"-P", "2", "-n", "1", "echo"}, "a b c", runner)
	if result.exitCode != 0 || result.stdout != "a\nb\nc\n" {
		t.Fatalf("result=%+v", result)
	}
	if maximum.Load() != 2 {
		t.Fatalf("maximum concurrency=%d", maximum.Load())
	}
}

func TestXargsPropagatesLastFailure(t *testing.T) {
	runner := func(_ context.Context, argv []string, commandCtx *command.Context) int {
		if argv[len(argv)-1] == "bad" {
			fmt.Fprintln(commandCtx.Stderr, "failed")
			return 7
		}
		fmt.Fprintln(commandCtx.Stdout, argv[len(argv)-1])
		return 0
	}
	result := runXargsTest(t, []string{"-n", "1", "check"}, "ok bad later", runner)
	if result.exitCode != 7 || result.stdout != "ok\nlater\n" || result.stderr != "failed\n" {
		t.Fatalf("result=%+v", result)
	}
}

type xargsTestResult struct {
	stdout   string
	stderr   string
	exitCode int
}

func runXargsTest(t *testing.T, args []string, stdin string, runner func(context.Context, []string, *command.Context) int) xargsTestResult {
	t.Helper()
	cwd := "/"
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	commandCtx := &command.Context{
		Cwd:        &cwd,
		Env:        map[string]string{},
		Stdin:      strings.NewReader(stdin),
		Stdout:     &stdout,
		Stderr:     &stderr,
		RunCommand: runner,
	}
	exitCode := commandXargs(context.Background(), args, commandCtx)
	return xargsTestResult{stdout: stdout.String(), stderr: stderr.String(), exitCode: exitCode}
}

func echoRunner(_ context.Context, argv []string, commandCtx *command.Context) int {
	fmt.Fprintln(commandCtx.Stdout, strings.Join(argv[1:], " "))
	return 0
}
