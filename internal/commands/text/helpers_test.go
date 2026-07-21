package text

import (
	"bytes"
	"context"
	"testing"

	"github.com/rumpl/gash/internal/command"
	gfs "github.com/rumpl/gash/pkg/fs"
)

type commandFunc func(context.Context, []string, *command.Context) int

func assertCommand(t *testing.T, run commandFunc, args []string, stdin, stdout string, files map[string]string) {
	t.Helper()
	filesystem := gfs.NewMemory(0)
	_ = filesystem.MkdirAll("work", 0o755)
	for name, content := range files {
		if err := filesystem.WriteFile("work/"+name, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var out, stderr bytes.Buffer
	cwd := "/work"
	ctx := &command.Context{FS: filesystem, Cwd: &cwd, Env: map[string]string{}, Stdin: bytes.NewBufferString(stdin), Stdout: &out, Stderr: &stderr}
	if code := run(context.Background(), args, ctx); code != 0 {
		t.Fatalf("exit=%d stderr=%q", code, stderr.String())
	}
	if out.String() != stdout {
		t.Fatalf("stdout=%q, want %q", out.String(), stdout)
	}
}
