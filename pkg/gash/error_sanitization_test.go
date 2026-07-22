package gash

import (
	"context"
	"strings"
	"testing"

	gashfs "github.com/rumpl/gash/pkg/fs"
)

func TestCommandErrorsDoNotExposeRootedHostPaths(t *testing.T) {
	root := t.TempDir()
	filesystem, err := gashfs.NewRooted(root)
	if err != nil {
		t.Fatal(err)
	}
	shell, err := New(Options{FS: filesystem, Cwd: "/"})
	if err != nil {
		t.Fatal(err)
	}
	result := shell.Exec(context.Background(), `cat /no/such/file`, ExecOptions{})
	if result.ExitCode != 1 {
		t.Fatalf("result=%+v", result)
	}
	if strings.Contains(result.Stderr, root) || strings.Contains(result.Stderr, "lstat") {
		t.Fatalf("stderr exposes filesystem implementation: %q", result.Stderr)
	}
	if result.Stderr != "cat: /no/such/file: No such file or directory\n" {
		t.Fatalf("stderr=%q", result.Stderr)
	}
}
