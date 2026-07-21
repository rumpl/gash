package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/rumpl/gash/pkg/gash"
)

type envFlags map[string]string

func (e envFlags) String() string {
	return "KEY=VALUE"
}

func (e envFlags) Set(value string) error {
	p := strings.SplitN(value, "=", 2)
	if len(p) != 2 || p[0] == "" {
		return fmt.Errorf("environment must be KEY=VALUE")
	}
	e[p[0]] = p[1]
	return nil
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	set := flag.NewFlagSet("gash", flag.ContinueOnError)
	set.SetOutput(stderr)
	command := set.String("c", "", "shell script to execute")
	jsonOutput := set.Bool("json", false, "print the result as JSON")
	cwd := set.String("cwd", "", "virtual working directory (defaults to / with --root)")
	root := set.String("root", "", "expose a host directory read-only as the virtual filesystem root")
	env := envFlags{}
	set.Var(env, "e", "set an environment variable (repeatable)")
	set.Usage = func() {
		fmt.Fprintln(set.Output(), "Usage: gash [-c script | file] [options]")
		set.PrintDefaults()
	}
	if err := set.Parse(args); err != nil {
		return 2
	}

	script, scriptName, scriptArgs, exitCode := readScript(*command, set.Args(), stdin, stderr)
	if exitCode != 0 {
		return exitCode
	}

	options := gash.Options{
		Cwd: *cwd,
		Env: env,
	}
	if *root != "" {
		absoluteRoot, err := filepath.Abs(*root)
		if err != nil {
			fmt.Fprintln(stderr, "gash: resolve root:", err)
			return 1
		}
		info, err := os.Stat(absoluteRoot)
		if err != nil {
			fmt.Fprintln(stderr, "gash: root:", err)
			return 1
		}
		if !info.IsDir() {
			fmt.Fprintf(stderr, "gash: root: %s is not a directory\n", *root)
			return 1
		}
		options.FS = os.DirFS(absoluteRoot)
		if options.Cwd == "" {
			options.Cwd = "/"
		}
	}

	bash, err := gash.New(options)
	if err != nil {
		fmt.Fprintln(stderr, "gash:", err)
		return 1
	}
	result := bash.Exec(context.Background(), script, gash.ExecOptions{Args: scriptArgs, ScriptName: scriptName})
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(result); err != nil {
			fmt.Fprintln(stderr, "gash:", err)
			return 1
		}
	} else {
		fmt.Fprint(stdout, result.Stdout)
		fmt.Fprint(stderr, result.Stderr)
	}
	return result.ExitCode
}

func readScript(command string, args []string, stdin io.Reader, stderr io.Writer) (script, scriptName string, scriptArgs []string, exitCode int) {
	if command != "" {
		return command, "", args, 0
	}
	if len(args) > 0 {
		data, err := os.ReadFile(args[0])
		if err != nil {
			fmt.Fprintln(stderr, "gash:", err)
			return "", "", nil, 1
		}
		return string(data), args[0], args[1:], 0
	}
	data, err := io.ReadAll(stdin)
	if err != nil {
		fmt.Fprintln(stderr, "gash:", err)
		return "", "", nil, 1
	}
	return string(data), "", nil, 0
}
