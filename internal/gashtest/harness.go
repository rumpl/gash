// Package gashtest provides a Go-native compatibility harness for running
// small shell scripts through pkg/gash and asserting their observable behavior.
//
// The fixtures in this package are Go-owned adaptations of representative
// public just-bash Bash execution tests pinned at commit
// 2b316eb26b3f3e832e2cf3994d4fef160d5eb8e4. They are intentionally smoke-sized:
// future command/language tasks can add cases here without building a full
// differential runner.
package gashtest

import (
	"context"
	iofs "io/fs"
	"path"
	"sort"
	"strings"
	"testing"
	"time"

	gfs "github.com/rumpl/gash/pkg/fs"
	"github.com/rumpl/gash/pkg/gash"
)

// Case describes one compatibility scenario to execute through gash.
//
// Fields cover the execution options exposed by gash (environment, working
// directory, stdin, args, and limits) plus filesystem setup and post-run
// assertions. Zero values are deliberately useful: stdout/stderr default to
// empty strings, status defaults to 0, and cwd defaults to gash's /home/user.
type Case struct {
	Name       string
	Script     string
	Files      map[string]string
	Env        map[string]string
	Cwd        string
	Stdin      string
	Args       []string
	ReplaceEnv bool
	Limits     gash.Limits
	Timeout    time.Duration

	WantStdout  string
	WantStderr  string
	WantStatus  int
	WantFiles   map[string]string
	WantMissing []string
	Check       func(t testing.TB, shell *gash.Bash, result gash.Result)
}

// Run executes tc in a subtest and asserts configured outputs, status, and
// filesystem effects. Callers may supply Check for richer assertions.
func Run(t *testing.T, tc Case) {
	t.Helper()
	name := tc.Name
	if name == "" {
		name = "compatibility case"
	}
	t.Run(name, func(t *testing.T) {
		t.Helper()
		shell, err := gash.New(gash.Options{Files: tc.Files, Env: tc.Env, Cwd: tc.Cwd, Limits: tc.Limits})
		if err != nil {
			t.Fatalf("new gash: %v", err)
		}
		ctx := context.Background()
		if tc.Timeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, tc.Timeout)
			defer cancel()
		}
		result := shell.Exec(ctx, tc.Script, gash.ExecOptions{Cwd: tc.Cwd, Stdin: tc.Stdin, Env: tc.Env, Args: tc.Args, ReplaceEnv: tc.ReplaceEnv})
		if result.ExitCode != tc.WantStatus {
			t.Fatalf("exit code = %d, want %d\nstdout=%q\nstderr=%q", result.ExitCode, tc.WantStatus, result.Stdout, result.Stderr)
		}
		if result.Stdout != tc.WantStdout {
			t.Fatalf("stdout = %q, want %q\nstderr=%q", result.Stdout, tc.WantStdout, result.Stderr)
		}
		if result.Stderr != tc.WantStderr {
			t.Fatalf("stderr = %q, want %q\nstdout=%q", result.Stderr, tc.WantStderr, result.Stdout)
		}
		for file, want := range tc.WantFiles {
			got, err := ReadFile(shell, file)
			if err != nil {
				t.Fatalf("read %s: %v", file, err)
			}
			if got != want {
				t.Fatalf("file %s = %q, want %q", file, got, want)
			}
		}
		for _, file := range tc.WantMissing {
			if Exists(shell, file) {
				t.Fatalf("file %s exists, want missing", file)
			}
		}
		if tc.Check != nil {
			tc.Check(t, shell, result)
		}
	})
}

// RunAll executes all cases using Run. It is the common entry point for compact
// table-driven compatibility suites.
func RunAll(t *testing.T, cases []Case) {
	t.Helper()
	for _, tc := range cases {
		Run(t, tc)
	}
}

// ReadFile reads a virtual path from the shell filesystem and returns it as a
// string. Tests can use it from Case.Check for custom filesystem assertions.
func ReadFile(shell *gash.Bash, name string) (string, error) {
	data, err := gfs.ReadFile(shell.FS, name)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// Exists reports whether a virtual path exists.
func Exists(shell *gash.Bash, name string) bool {
	_, err := gfs.Stat(shell.FS, name)
	return err == nil
}

// Tree returns a stable, newline-joined listing of all paths beneath root. It is
// useful for checking filesystem mutation shape without depending on host state.
func Tree(shell *gash.Bash, root string) (string, error) {
	root = path.Clean("/" + strings.TrimPrefix(root, "/"))
	var paths []string
	if err := walk(shell.FS, root, &paths); err != nil {
		return "", err
	}
	sort.Strings(paths)
	return strings.Join(paths, "\n"), nil
}

func walk(fsys iofs.FS, name string, paths *[]string) error {
	info, err := gfs.Stat(fsys, name)
	if err != nil {
		return err
	}
	*paths = append(*paths, name)
	if !info.IsDir() {
		return nil
	}
	entries, err := gfs.ReadDir(fsys, name)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		child := path.Join(name, entry.Name())
		if name == "/" {
			child = "/" + entry.Name()
		}
		if err := walk(fsys, child, paths); err != nil {
			return err
		}
	}
	return nil
}

// WantFileMode returns a Check function that asserts a virtual path's mode.
func WantFileMode(name string, mode iofs.FileMode) func(testing.TB, *gash.Bash, gash.Result) {
	return func(t testing.TB, shell *gash.Bash, _ gash.Result) {
		t.Helper()
		info, err := gfs.Stat(shell.FS, name)
		if err != nil {
			t.Fatalf("stat %s: %v", name, err)
		}
		if got := info.Mode().Perm(); got != mode.Perm() {
			t.Fatalf("mode %s = %v, want %v", name, got, mode.Perm())
		}
	}
}

// WantQuotaError can be used by cases that intentionally trip filesystem
// limits while avoiding exact stderr wording in the table.
func WantQuotaError(t testing.TB, _ *gash.Bash, result gash.Result) {
	t.Helper()
	if !strings.Contains(result.Stderr, gfs.ErrQuota.Error()) {
		t.Fatalf("stderr %q does not contain quota error", result.Stderr)
	}
}
