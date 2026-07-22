package main

import (
	"context"
	"fmt"
	"os"

	"github.com/rumpl/gash/pkg/gash"
)

func main() {
	shell, err := gash.New(gash.Options{
		Files: map[string]string{
			"/work/names.txt": "Ada\nGrace\nLinus\n",
		},
		Env: map[string]string{
			"GREETING": "hello",
		},
		Cwd:          "/work",
		LimitProfile: gash.HardenedProfile,
	})
	if err != nil {
		panic(err)
	}

	result := shell.Exec(
		context.Background(),
		`printf '%s, %s!\n' "$GREETING" "$1"; grep "$2" names.txt`,
		gash.ExecOptions{
			Args:       []string{"developer", "Grace"},
			ScriptName: "welcome.sh",
		},
	)
	fmt.Print(result.Stdout)
	fmt.Fprint(os.Stderr, result.Stderr)
	if result.ExitCode != 0 {
		os.Exit(result.ExitCode)
	}
}
