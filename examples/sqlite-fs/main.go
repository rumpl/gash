// Command sqlite-fs stores the whole virtual filesystem in a SQLite database
// and copy-mounts a host directory into it. Run it twice: the second run finds
// the files written by the first one, without touching the host directory
// again.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	gashfs "github.com/rumpl/gash/pkg/fs"
	"github.com/rumpl/gash/pkg/gash"
)

func main() {
	database := flag.String("db", filepath.Join(os.TempDir(), "gash-fs.db"), "SQLite database file backing the filesystem")
	source := flag.String("mount", ".", "host directory copied into the filesystem at /project")
	replace := flag.Bool("replace", false, "copy the host directory again, discarding earlier changes")
	flag.Parse()

	policy := gashfs.MountIfAbsent
	if *replace {
		policy = gashfs.MountReplace
	}
	filesystem, err := gashfs.NewSQLite(gashfs.SQLiteOptions{
		Path: *database,
		Mounts: []gashfs.CopyMount{{
			HostPath: *source,
			Target:   "/project",
			Policy:   policy,
		}},
	})
	if err != nil {
		panic(err)
	}
	defer filesystem.Close()

	shell, err := gash.New(gash.Options{FS: filesystem, Cwd: "/project"})
	if err != nil {
		panic(err)
	}
	result := shell.Exec(context.Background(), `
		mkdir -p /project/.runs
		date +%s >> /project/.runs/history
		printf 'files copied from the host: %s\n' "$(find /project -type f -not -path '*/.runs/*' | wc -l)"
		printf 'recorded runs: %s\n' "$(wc -l < /project/.runs/history)"
	`, gash.ExecOptions{})
	fmt.Print(result.Stdout)
	fmt.Fprint(os.Stderr, result.Stderr)
	fmt.Printf("database: %s (%d bytes of file data)\n", *database, filesystem.Used())
	if result.ExitCode != 0 {
		os.Exit(result.ExitCode)
	}
}
