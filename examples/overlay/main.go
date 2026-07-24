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
		"config/app.conf":   &fstest.MapFile{Data: []byte("mode=production\n")},
		"config/legacy.ini": &fstest.MapFile{Data: []byte("[legacy]\n")},
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
		`printf 'lower layer: '; cat /config/app.conf
# Appending copies the lower-only file into the upper layer first.
printf 'debug=true\n' >> /config/app.conf
printf 'after copy-up: '; tr '\n' ' ' < /config/app.conf; echo
# Deleting a lower-only file records a whiteout in the upper layer.
rm /config/legacy.ini
printf 'remaining config entries: '; ls /config
mkdir -p /output; printf 'temporary\n' > /output/state.txt; cat /output/state.txt`,
		gash.ExecOptions{},
	)
	fmt.Print(result.Stdout)
	if data, err := lower.ReadFile("config/legacy.ini"); err == nil {
		fmt.Printf("lower layer is untouched: config/legacy.ini still holds %q\n", data)
	}
	fmt.Fprint(os.Stderr, result.Stderr)
	if result.ExitCode != 0 {
		os.Exit(result.ExitCode)
	}
}
