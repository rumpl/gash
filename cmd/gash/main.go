package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/rumpl/gash/pkg/gash"
)

type envFlags map[string]string

func (e envFlags) String() string { return "KEY=VALUE" }
func (e envFlags) Set(value string) error {
	p := strings.SplitN(value, "=", 2)
	if len(p) != 2 || p[0] == "" {
		return fmt.Errorf("environment must be KEY=VALUE")
	}
	e[p[0]] = p[1]
	return nil
}

func main() { os.Exit(run()) }
func run() int {
	set := flag.NewFlagSet("gash", flag.ContinueOnError)
	set.SetOutput(os.Stderr)
	command := set.String("c", "", "shell script to execute")
	jsonOutput := set.Bool("json", false, "print the result as JSON")
	cwd := set.String("cwd", "/home/user", "virtual working directory")
	env := envFlags{}
	set.Var(env, "e", "set an environment variable (repeatable)")
	set.Usage = func() { fmt.Fprintln(set.Output(), "Usage: gash [-c script | file] [options]"); set.PrintDefaults() }
	if err := set.Parse(os.Args[1:]); err != nil {
		return 2
	}
	script := *command
	if script == "" && set.NArg() > 0 {
		data, err := os.ReadFile(set.Arg(0))
		if err != nil {
			fmt.Fprintln(os.Stderr, "gash:", err)
			return 1
		}
		script = string(data)
	} else if script == "" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintln(os.Stderr, "gash:", err)
			return 1
		}
		script = string(data)
	}
	bash, err := gash.New(gash.Options{Cwd: *cwd, Env: env})
	if err != nil {
		fmt.Fprintln(os.Stderr, "gash:", err)
		return 1
	}
	result := bash.Exec(context.Background(), script, gash.ExecOptions{})
	if *jsonOutput {
		if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
			fmt.Fprintln(os.Stderr, "gash:", err)
			return 1
		}
	} else {
		fmt.Fprint(os.Stdout, result.Stdout)
		fmt.Fprint(os.Stderr, result.Stderr)
	}
	return result.ExitCode
}
