package gash

import (
	"context"
	"strings"
	"testing"
	"testing/fstest"
)

func TestNoClobberAndForcedClobberRedirections(t *testing.T) {
	shell := newTestBash(t)
	result := shell.Exec(context.Background(), `
printf old > file
set -C
printf new > file
printf 'status=%s\n' "$?"
cat file
printf force >| file
cat file
set +C
printf disabled > file
cat file
`, ExecOptions{})
	if result.ExitCode != 0 || result.Stdout != "status=1\noldforcedisabled" || !strings.Contains(result.Stderr, "exists") {
		t.Fatalf("result=%+v", result)
	}

	result = shell.Exec(context.Background(), `printf old > long; set -o noclobber; printf new > long; echo status=$?; set +o noclobber; printf ok > long; cat long`, ExecOptions{})
	if result.ExitCode != 0 || result.Stdout != "status=1\nok" || !strings.Contains(result.Stderr, "exists") {
		t.Fatalf("long option result=%+v", result)
	}
}

func TestForcedClobberOnReadOnlyFilesystemIsNormalCommandFailure(t *testing.T) {
	filesystem := fstest.MapFS{
		".":         {Mode: 0o555 | 0x80000000},
		"README.md": {Data: []byte("docs"), Mode: 0o444},
	}
	shell, err := New(Options{FS: filesystem})
	if err != nil {
		t.Fatal(err)
	}
	result := shell.Exec(context.Background(), `echo x >| /README.md; echo after`, ExecOptions{})
	if result.ExitCode != 0 || result.Stdout != "after\n" || !strings.Contains(result.Stderr, "filesystem is read-only") || strings.Contains(result.Stderr, "interpreter failure") {
		t.Fatalf("result=%+v", result)
	}
}
