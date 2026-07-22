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
			"/data/.keep": "",
		},
		Cwd: "/data",
	})
	if err != nil {
		panic(err)
	}

	create := shell.Exec(
		context.Background(),
		`sqlite3 people.db "CREATE TABLE people(name TEXT); INSERT INTO people VALUES ('Ada'), ('Grace');"`,
		gash.ExecOptions{},
	)
	if create.ExitCode != 0 {
		fmt.Fprint(os.Stderr, create.Stderr)
		os.Exit(create.ExitCode)
	}

	query := shell.Exec(
		context.Background(),
		`sqlite3 -header -column people.db "SELECT rowid, name FROM people ORDER BY rowid;"`,
		gash.ExecOptions{},
	)
	fmt.Print(query.Stdout)
	fmt.Fprint(os.Stderr, query.Stderr)
	if query.ExitCode != 0 {
		os.Exit(query.ExitCode)
	}
}
