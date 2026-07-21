package gash

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	gfs "github.com/rumpl/gash/pkg/fs"
	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
)

func (b *Bash) Exec(parent context.Context, script string, options ExecOptions) Result {
	b.mu.Lock()
	defer b.mu.Unlock()
	env := executionEnv(b.env)
	if options.ReplaceEnv {
		env = executionEnv(nil)
	}
	for k, v := range options.Env {
		env[k] = v
	}
	enforceInternalEnv(env)
	cwd := b.cwd
	if options.Cwd != "" {
		cwd = options.Cwd
	}
	env["PWD"] = cwd
	ctx, cancelTimeout := context.WithTimeout(parent, b.limits.MaxExecutionTime)
	defer cancelTimeout()
	ctx, cancelOutput := context.WithCancel(ctx)
	defer cancelOutput()
	budget := &outputBudget{maximum: int64(b.limits.MaxOutputBytes), cancel: cancelOutput}
	out := &boundedBuffer{budget: budget}
	errout := &boundedBuffer{budget: budget}
	scope := &executionScope{limits: b.limits}
	code, finalEnv := b.execute(ctx, script, options.Stdin, cwd, env, options.Args, options.ScriptName, out, errout, 0, scope, false)
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		fmt.Fprintln(errout, "bash: execution timed out")
		code = 124
	}
	if budget.exceeded.Load() {
		fmt.Fprintln(errout, "bash: output size limit exceeded")
		code = 126
	}
	return Result{Stdout: out.String(), Stderr: errout.String(), ExitCode: code, Env: finalEnv}
}

func (b *Bash) execute(ctx context.Context, script, stdin, cwd string, env map[string]string, args []string, scriptName string, stdout, stderr io.Writer, depth int, scope *executionScope, stdinAccounted bool) (int, map[string]string) {
	if depth > b.limits.MaxExecDepth {
		fmt.Fprintf(stderr, "bash: maximum nested execution depth (%d) exceeded\n", b.limits.MaxExecDepth)
		return 126, env
	}
	if int64(len(script)) > b.limits.MaxSourceBytes {
		fmt.Fprintf(stderr, "bash: source size limit exceeded (%d bytes)\n", b.limits.MaxSourceBytes)
		return 126, env
	}
	if !stdinAccounted {
		if err := scope.consumeInput(int64(len(stdin))); err != nil {
			fmt.Fprintf(stderr, "bash: %v\n", err)
			return 126, env
		}
	}
	parser := syntax.NewParser(syntax.Variant(syntax.LangBash))
	program, err := parser.Parse(strings.NewReader(script), scriptName)
	if err != nil {
		fmt.Fprintf(stderr, "bash: %v\n", err)
		return 2, env
	}
	virtualizeHostParameters(program)
	if err := rejectHostBackedSyntax(program); err != nil {
		fmt.Fprintf(stderr, "bash: %v\n", err)
		return 2, env
	}
	pairs := make([]string, 0, len(env))
	for k, v := range env {
		pairs = append(pairs, k+"="+v)
	}
	runner, err := interp.New(interp.Env(expand.ListEnviron(pairs...)), interp.Params(args...), interp.StdIO(strings.NewReader(stdin), stdout, stderr), interp.OpenHandler(b.openHandler), interp.ReadDirHandler2(b.readDirHandler), interp.StatHandler(b.statHandler), interp.CallHandler(func(callCtx context.Context, argv []string) ([]string, error) {
		if err := scope.chargeCommand(); err != nil {
			fmt.Fprintf(interp.HandlerCtx(callCtx).Stderr, "bash: %v\n", err)
			return argv, interp.NewExitStatus(126)
		}
		return argv, nil
	}), interp.ExecHandler(func(callCtx context.Context, argv []string) error { return b.execCommand(callCtx, argv, depth, scope) }))
	if err != nil {
		fmt.Fprintf(stderr, "bash: %v\n", err)
		return 1, env
	}
	// interp.Dir validates against the host filesystem. Setting the exported
	// field keeps cwd validation and all later access inside our io/fs handlers.
	runner.Dir = cwd
	err = runner.Run(ctx, program)
	code := 0
	if status, ok := interp.IsExitStatus(err); ok {
		code = int(status)
	} else if err != nil {
		if ctx.Err() != nil {
			code = 124
		} else {
			fmt.Fprintf(stderr, "bash: %v\n", err)
			code = 1
		}
	}
	final := cloneMap(env)
	for name, v := range runner.Vars {
		if v.IsSet() {
			final[name] = v.String()
		} else {
			delete(final, name)
		}
	}
	final["PWD"] = runner.Dir
	return code, final
}

func (b *Bash) execCommand(ctx context.Context, args []string, depth int, scope *executionScope) error {
	h := interp.HandlerCtx(ctx)
	if len(args) == 0 {
		return nil
	}
	name := strings.TrimPrefix(strings.TrimPrefix(args[0], "/bin/"), "/usr/bin/")
	env := map[string]string{}
	h.Env.Each(func(k string, v expand.Variable) bool {
		if v.IsSet() && !isHiddenInternalEnv(k) {
			env[k] = v.String()
		}
		return true
	})
	enforcePublicInternalEnv(env)
	cwd := h.Dir
	if name == "bash" || name == "sh" {
		if depth >= b.limits.MaxCallDepth {
			fmt.Fprintln(h.Stderr, "bash: maximum call depth exceeded")
			return interp.NewExitStatus(126)
		}
		argv := args[1:]
		var script string
		var params []string
		scriptName := ""
		if len(argv) >= 2 && argv[0] == "-c" {
			script = argv[1]
			if len(argv) > 2 {
				scriptName = argv[2]
				params = argv[3:]
			}
		} else if len(argv) > 0 {
			data, e := gfs.ReadFile(b.FS, resolve(cwd, argv[0]))
			if e != nil {
				fmt.Fprintf(h.Stderr, "%s: %v\n", name, e)
				return interp.NewExitStatus(1)
			}
			script = string(data)
			scriptName = argv[0]
			params = argv[1:]
		} else {
			data, _ := io.ReadAll(h.Stdin)
			script = string(data)
		}
		data, _ := io.ReadAll(h.Stdin)
		code, _ := b.execute(ctx, script, string(data), cwd, env, params, scriptName, h.Stdout, h.Stderr, depth+1, scope, true)
		if code != 0 {
			return interp.NewExitStatus(uint8(code))
		}
		return nil
	}
	cmd, ok := b.commands[name]
	if !ok {
		fmt.Fprintf(h.Stderr, "bash: %s: command not found\n", name)
		return interp.NewExitStatus(127)
	}
	code := cmd.Run(ctx, args[1:], &CommandContext{FS: b.FS, Cwd: &cwd, Env: env, Stdin: h.Stdin, Stdout: h.Stdout, Stderr: h.Stderr})
	if code != 0 {
		return interp.NewExitStatus(uint8(code))
	}
	return nil
}
