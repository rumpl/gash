package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	gashfs "github.com/rumpl/gash/pkg/fs"
	"github.com/rumpl/gash/pkg/gash"
)

func main() {
	lowerPath := flag.String("lower", ".", "read-only lower host directory")
	upperPath := flag.String("upper", "", "directory that persists the writable overlay diff")
	reset := flag.Bool("reset", false, "delete the persisted upper layer before running")
	flag.Parse()

	lowerRoot, err := filepath.Abs(*lowerPath)
	if err != nil {
		panic(err)
	}
	upperRoot, err := persistentUpperPath(*upperPath)
	if err != nil {
		panic(err)
	}
	if *reset {
		if err := os.RemoveAll(upperRoot); err != nil {
			panic(err)
		}
	}
	if err := os.MkdirAll(upperRoot, 0o755); err != nil {
		panic(err)
	}

	upper, err := gashfs.NewRooted(upperRoot)
	if err != nil {
		panic(err)
	}
	filesystem, err := gashfs.NewOverlay(gashfs.OverlayOptions{
		Upper: upper,
		Lower: os.DirFS(lowerRoot),
	})
	if err != nil {
		panic(err)
	}
	shell, err := gash.New(gash.Options{
		FS:  filesystem,
		Cwd: "/",
		Env: map[string]string{
			"RUN_AT": time.Now().UTC().Format(time.RFC3339Nano),
		},
	})
	if err != nil {
		panic(err)
	}

	result := shell.Exec(
		context.Background(),
		`mkdir -p /state
printf '%s\n' "$RUN_AT" >> /state/runs.log
printf 'number of runs using this upper layer: '
wc -l < /state/runs.log
printf 'most recent runs:\n'
tail -n 3 /state/runs.log`,
		gash.ExecOptions{},
	)
	fmt.Printf("lower layer: %s\n", lowerRoot)
	fmt.Printf("persisted diff: %s\n", upperRoot)
	fmt.Print(result.Stdout)
	fmt.Fprint(os.Stderr, result.Stderr)
	if result.ExitCode != 0 {
		os.Exit(result.ExitCode)
	}
}

func persistentUpperPath(configured string) (string, error) {
	if configured != "" {
		return filepath.Abs(configured)
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cache, "gash", "persistent-overlay-example"), nil
}
