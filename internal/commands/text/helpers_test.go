package text

import (
	"bytes"
	"context"
	"path"
	"testing"

	"github.com/rumpl/gash/internal/command"
	gfs "github.com/rumpl/gash/pkg/fs"
)

type commandFunc func(context.Context, []string, *command.Context) int

func assertCommand(t *testing.T, run commandFunc, args []string, stdin, stdout string, files map[string]string) {
	t.Helper()
	stringFiles := map[string][]byte{}
	for name, content := range files {
		stringFiles[name] = []byte(content)
	}
	assertCommandBytes(t, run, args, []byte(stdin), []byte(stdout), stringFiles)
}

func assertCommandBytes(t *testing.T, run commandFunc, args []string, stdin, wantStdout []byte, files map[string][]byte) {
	t.Helper()
	code, stdout, stderr, _ := runTextCommandBytes(t, run, args, stdin, files)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%q", code, stderr)
	}
	if !bytes.Equal(stdout, wantStdout) {
		t.Fatalf("stdout=%q, want %q", stdout, wantStdout)
	}
}

func runTextCommandBytes(t *testing.T, run commandFunc, args []string, stdin []byte, files map[string][]byte) (int, []byte, string, *gfs.Memory) {
	t.Helper()
	filesystem := gfs.NewMemory(0)
	_ = filesystem.MkdirAll("work", 0o755)
	for name, content := range files {
		dir := path.Dir(name)
		if dir != "." {
			if err := filesystem.MkdirAll("work/"+dir, 0o755); err != nil {
				t.Fatal(err)
			}
		}
		if err := filesystem.WriteFile("work/"+name, content, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var out, stderr bytes.Buffer
	cwd := "/work"
	ctx := &command.Context{FS: filesystem, Cwd: &cwd, Env: map[string]string{}, Stdin: bytes.NewReader(stdin), Stdout: &out, Stderr: &stderr}
	code := run(context.Background(), args, ctx)
	return code, out.Bytes(), stderr.String(), filesystem
}
