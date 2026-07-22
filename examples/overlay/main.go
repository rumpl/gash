package main

import (
	"context"
	"fmt"
	"os"
	"testing/fstest"

	gashfs "github.com/rumpl/gash/pkg/fs"
	"github.com/rumpl/gash/pkg/gash"
)

func main() {
	lower := fstest.MapFS{
		"config/app.conf": &fstest.MapFile{Data: []byte("mode=production\n")},
	}
	upper := gashfs.NewMemory(8 << 20)
	filesystem, err := gashfs.NewOverlay(gashfs.OverlayOptions{
		Upper: upper,
		Lower: lower,
	})
	if err != nil {
		panic(err)
	}
	shell, err := gash.New(gash.Options{
		FS:  filesystem,
		Cwd: "/",
	})
	if err != nil {
		panic(err)
	}
	result := shell.Exec(
		context.Background(),
		`cat /config/app.conf; mkdir -p /output; printf 'temporary\n' > /output/state.txt; cat /output/state.txt`,
		gash.ExecOptions{},
	)
	fmt.Print(result.Stdout)
	fmt.Fprint(os.Stderr, result.Stderr)
	if result.ExitCode != 0 {
		os.Exit(result.ExitCode)
	}
}
