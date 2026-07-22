package gash

import (
	"context"
	"testing"

	gfs "github.com/rumpl/gash/pkg/fs"
)

func TestReadablePredicateUsesFilesystemReadCapability(t *testing.T) {
	memory := gfs.NewMemory(1024)
	if err := memory.WriteFile("README.md", []byte("readable"), 0o644); err != nil {
		t.Fatal(err)
	}
	shell, err := New(Options{FS: gfs.ReadOnly(memory)})
	if err != nil {
		t.Fatal(err)
	}
	result := shell.Exec(context.Background(), `
[ -r README.md ]; echo bracket=$?
test -r README.md; echo test=$?
[[ -r README.md ]]; echo extended=$?
[ -r . ]; echo directory=$?
[ -r missing ]; echo missing=$?
[ ! -r README.md ]; echo negated=$?
cat README.md
`, ExecOptions{})
	want := "bracket=0\ntest=0\nextended=0\ndirectory=0\nmissing=1\nnegated=1\nreadable"
	if result.ExitCode != 0 || result.Stdout != want || result.Stderr != "" {
		t.Fatalf("result=%+v", result)
	}
}
