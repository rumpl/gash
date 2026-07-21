package commands

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/rumpl/gash/internal/command"
	"github.com/rumpl/gash/internal/commandhelp"
	gfs "github.com/rumpl/gash/pkg/fs"
)

func TestEveryUpstreamHelpDefinitionIsEnforced(t *testing.T) {
	for _, builtin := range Builtins() {
		info, hasHelp := commandhelp.Lookup(builtin.Name)
		if !hasHelp {
			continue
		}
		t.Run(builtin.Name, func(t *testing.T) {
			filesystem := gfs.NewMemory(0)
			if err := filesystem.MkdirAll("work", 0o755); err != nil {
				t.Fatal(err)
			}
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
			if exitCode := builtin.Run(context.Background(), []string{"--help"}, ctx); exitCode != 0 {
				t.Fatalf("exit=%d stderr=%q", exitCode, stderr.String())
			}
			if stderr.Len() != 0 {
				t.Fatalf("stderr=%q", stderr.String())
			}
			expectedPrefix := info.Name + " - " + info.Summary + "\n\n"
			if !strings.HasPrefix(stdout.String(), expectedPrefix) {
				t.Fatalf("help output does not start with %q:\n%s", expectedPrefix, stdout.String())
			}
		})
	}
}
