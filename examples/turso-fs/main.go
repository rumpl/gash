// Command turso-fs stores the virtual filesystem in a Turso (libSQL) database
// so several machines can share one shell filesystem. It needs a database URL
// and token:
//
//	export TURSO_DATABASE_URL=libsql://your-database-your-org.turso.io
//	export TURSO_AUTH_TOKEN=...
//	go run ./examples/turso-fs
//
// A local `turso dev --port 8080` server also works with
// TURSO_DATABASE_URL=http://127.0.0.1:8080 and no token.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	gashfs "github.com/rumpl/gash/pkg/fs"
	"github.com/rumpl/gash/pkg/gash"
)

func main() {
	url := flag.String("url", os.Getenv("TURSO_DATABASE_URL"), "Turso database URL")
	token := flag.String("token", os.Getenv("TURSO_AUTH_TOKEN"), "Turso auth token")
	source := flag.String("mount", "", "optional host directory copied into the filesystem at /project")
	flag.Parse()

	if *url == "" {
		fmt.Fprintln(os.Stderr, "set TURSO_DATABASE_URL or pass -url")
		os.Exit(2)
	}
	var mounts []gashfs.CopyMount
	if *source != "" {
		mounts = append(mounts, gashfs.CopyMount{HostPath: *source, Target: "/project"})
	}
	filesystem, err := gashfs.NewTurso(gashfs.TursoOptions{
		URL:       *url,
		AuthToken: *token,
		Mounts:    mounts,
	})
	if err != nil {
		panic(err)
	}
	defer filesystem.Close()

	shell, err := gash.New(gash.Options{FS: filesystem, Cwd: "/"})
	if err != nil {
		panic(err)
	}
	result := shell.Exec(context.Background(), `
		mkdir -p /shared
		printf 'hello from %s\n' "$(hostname)" >> /shared/guestbook.txt
		cat /shared/guestbook.txt
	`, gash.ExecOptions{})
	fmt.Print(result.Stdout)
	fmt.Fprint(os.Stderr, result.Stderr)
	if result.ExitCode != 0 {
		os.Exit(result.ExitCode)
	}
}
