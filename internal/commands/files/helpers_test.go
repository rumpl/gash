package files

import (
	"bytes"
	"context"
	"io/fs"
	"testing"

	"github.com/rumpl/gash/internal/command"
	gfs "github.com/rumpl/gash/pkg/fs"
)

type commandFunc func(context.Context, []string, *command.Context) int

type commandResult struct {
	filesystem *gfs.Memory
	stdout     string
	stderr     string
	exitCode   int
}

func runCommand(t *testing.T, run commandFunc, args []string, files map[string]string) commandResult {
	t.Helper()
	filesystem := gfs.NewMemory(0)
	if err := filesystem.MkdirAll("work", 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range files {
		if err := filesystem.WriteFile("work/"+name, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return runCommandWithFS(t, run, args, filesystem)
}

func runCommandWithFS(t *testing.T, run commandFunc, args []string, filesystem *gfs.Memory) commandResult {
	t.Helper()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cwd := "/work"
	ctx := &command.Context{
		FS:     filesystem,
		Cwd:    &cwd,
		Env:    map[string]string{},
		Stdin:  &bytes.Buffer{},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	exitCode := run(context.Background(), args, ctx)
	return commandResult{filesystem: filesystem, stdout: stdout.String(), stderr: stderr.String(), exitCode: exitCode}
}

func exists(filesystem *gfs.Memory, name string) bool {
	_, err := filesystem.Stat(name)
	return err == nil
}

func mode(filesystem *gfs.Memory, name string) fs.FileMode {
	info, err := filesystem.Stat(name)
	if err != nil {
		return 0
	}
	return info.Mode().Perm()
}
