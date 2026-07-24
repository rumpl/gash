package gash_test

import (
	"context"
	"fmt"
	iofs "io/fs"
	"testing"
	"testing/fstest"

	gashfs "github.com/rumpl/gash/pkg/fs"
	"github.com/rumpl/gash/pkg/gash"
)

func TestPublicPackageCustomCommand(t *testing.T) {
	shell, err := gash.New(gash.Options{Commands: []gash.Command{{Name: "hello", Run: func(_ context.Context, args []string, c *gash.CommandContext) int {
		fmt.Fprintln(c.Stdout, "hello")
		return 0
	}}}})
	if err != nil {
		t.Fatal(err)
	}
	result := shell.Exec(context.Background(), "hello", gash.ExecOptions{})
	if result.ExitCode != 0 || result.Stdout != "hello\n" {
		t.Fatalf("%+v", result)
	}
}

func TestPublicPackageOverlayCopyUpAndWhiteout(t *testing.T) {
	lower := fstest.MapFS{
		"data/notes.txt": {Data: []byte("lower\n")},
		"data/gone.txt":  {Data: []byte("bye\n")},
		"data/keep.txt":  {Data: []byte("keep\n")},
	}
	upper := gashfs.NewMemory(1 << 20)
	filesystem, err := gashfs.NewOverlay(gashfs.OverlayOptions{Upper: upper, Lower: lower})
	if err != nil {
		t.Fatal(err)
	}
	shell, err := gash.New(gash.Options{FS: filesystem, Cwd: "/"})
	if err != nil {
		t.Fatal(err)
	}
	result := shell.Exec(context.Background(), `set -e
echo appended >> /data/notes.txt
rm /data/gone.txt
ls /data
cat /data/notes.txt`, gash.ExecOptions{})
	if result.ExitCode != 0 {
		t.Fatalf("%+v", result)
	}
	if result.Stdout != "keep.txt\nnotes.txt\nlower\nappended\n" {
		t.Fatalf("stdout=%q", result.Stdout)
	}
	if data, err := iofs.ReadFile(lower, "data/gone.txt"); err != nil || string(data) != "bye\n" {
		t.Fatalf("lower layer mutated: %q %v", data, err)
	}

	result = shell.Exec(context.Background(), `set -e
rm -rf /data
mkdir /data
ls -A /data
echo fresh > /data/new.txt
ls /data`, gash.ExecOptions{})
	if result.ExitCode != 0 {
		t.Fatalf("%+v", result)
	}
	if result.Stdout != "new.txt\n" {
		t.Fatalf("recreated directory stdout=%q", result.Stdout)
	}
}
