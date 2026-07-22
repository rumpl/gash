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
	base := gashfs.NewMemory(8 << 20)
	dataset := fstest.MapFS{
		"users.csv": &fstest.MapFile{Data: []byte("id,name\n1,Ada\n2,Grace\n")},
	}
	filesystem, err := gashfs.NewMountable(gashfs.MountableOptions{
		Base: base,
		Mounts: []gashfs.MountConfig{
			{Point: "/data", FS: dataset},
		},
	})
	if err != nil {
		panic(err)
	}
	shell, err := gash.New(gash.Options{
		FS:  filesystem,
		Cwd: "/home/user",
	})
	if err != nil {
		panic(err)
	}
	result := shell.Exec(
		context.Background(),
		`cut -d, -f2 /data/users.csv`,
		gash.ExecOptions{},
	)
	fmt.Print(result.Stdout)
	fmt.Fprint(os.Stderr, result.Stderr)
	if result.ExitCode != 0 {
		os.Exit(result.ExitCode)
	}
}
