// Package gash implements a bash-like shell over a virtual filesystem.
package gash

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"sync"
	"time"

	gfs "github.com/rumpl/gash/fs"
)

type Result struct {
	Stdout   string            `json:"stdout"`
	Stderr   string            `json:"stderr"`
	ExitCode int               `json:"exitCode"`
	Env      map[string]string `json:"env,omitempty"`
}
type Limits struct {
	MaxCommands        int
	MaxOutputBytes     int
	MaxExecutionTime   time.Duration
	MaxFileSystemBytes int64
}
type Options struct {
	FS       gfs.FileSystem
	Files    map[string]string
	Env      map[string]string
	Cwd      string
	Limits   Limits
	Commands []Command
}
type ExecOptions struct {
	Env        map[string]string
	Cwd, Stdin string
	ReplaceEnv bool
}
type CommandFunc func(context.Context, []string, *CommandContext) int
type Command struct {
	Name string
	Run  CommandFunc
}
type CommandContext struct {
	FS             gfs.FileSystem
	Cwd            *string
	Env            map[string]string
	Stdin          io.Reader
	Stdout, Stderr io.Writer
}

type Bash struct {
	FS       gfs.FileSystem
	env      map[string]string
	cwd      string
	commands map[string]Command
	limits   Limits
	mu       sync.Mutex
}

func New(options Options) (*Bash, error) {
	limits := options.Limits
	if limits.MaxCommands == 0 {
		limits.MaxCommands = 20_000
	}
	if limits.MaxOutputBytes == 0 {
		limits.MaxOutputBytes = 32 << 20
	}
	if limits.MaxExecutionTime == 0 {
		limits.MaxExecutionTime = 30 * time.Second
	}
	if limits.MaxFileSystemBytes == 0 {
		limits.MaxFileSystemBytes = 1 << 30
	}
	filesystem := options.FS
	if filesystem == nil {
		filesystem = gfs.NewMemory(limits.MaxFileSystemBytes)
	}
	cwd := options.Cwd
	if cwd == "" {
		cwd = "/home/user"
	}
	_ = gfs.MkdirAll(filesystem, "/home/user", 0755)
	_ = gfs.MkdirAll(filesystem, "/tmp", 0777)
	_ = gfs.MkdirAll(filesystem, "/bin", 0755)
	_ = gfs.MkdirAll(filesystem, "/usr/bin", 0755)
	for p, data := range options.Files {
		abs := resolve("/", p)
		_ = gfs.MkdirAll(filesystem, pathDir(abs), 0755)
		if err := gfs.WriteFile(filesystem, abs, []byte(data), 0644); err != nil {
			return nil, err
		}
	}
	env := map[string]string{"HOME": "/home/user", "PATH": "/usr/bin:/bin", "PWD": cwd, "OLDPWD": cwd, "HOSTNAME": "localhost", "USER": "user", "IFS": " \t\n"}
	for k, v := range options.Env {
		env[k] = v
	}
	b := &Bash{FS: filesystem, env: env, cwd: cwd, commands: map[string]Command{}, limits: limits}
	for _, c := range builtinCommands() {
		b.RegisterCommand(c)
	}
	for _, c := range options.Commands {
		b.RegisterCommand(c)
	}
	return b, nil
}
func resolve(base, name string) string {
	if strings.HasPrefix(name, "/") {
		return path.Clean(name)
	}
	return path.Clean(path.Join(base, name))
}

func pathDir(p string) string {
	i := strings.LastIndex(p, "/")
	if i <= 0 {
		return "/"
	}
	return p[:i]
}
func (b *Bash) RegisterCommand(c Command) { b.commands[c.Name] = c }
func cloneMap(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

type boundedBuffer struct {
	bytes.Buffer
	limit, total int
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if b.total+len(p) > b.limit {
		return 0, errors.New("output size limit exceeded")
	}
	n, e := b.Buffer.Write(p)
	b.total += n
	return n, e
}

func (b *Bash) Exec(ctx context.Context, script string, options ExecOptions) Result {
	b.mu.Lock()
	defer b.mu.Unlock()
	env := cloneMap(b.env)
	if options.ReplaceEnv {
		env = map[string]string{}
	}
	for k, v := range options.Env {
		env[k] = v
	}
	cwd := b.cwd
	if options.Cwd != "" {
		cwd = options.Cwd
	}
	env["PWD"] = cwd
	ctx, cancel := context.WithTimeout(ctx, b.limits.MaxExecutionTime)
	defer cancel()
	out := &boundedBuffer{limit: b.limits.MaxOutputBytes}
	errout := &boundedBuffer{limit: b.limits.MaxOutputBytes}
	code := b.execute(ctx, script, options.Stdin, &cwd, env, out, errout, 0)
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		fmt.Fprintln(errout, "bash: execution timed out")
		code = 124
	}
	return Result{Stdout: out.String(), Stderr: errout.String(), ExitCode: code, Env: env}
}
func (b *Bash) ReadFile(name string) (string, error) {
	data, e := gfs.ReadFile(b.FS, resolve(b.cwd, name))
	return string(data), e
}
func (b *Bash) WriteFile(name, data string) error {
	return gfs.WriteFile(b.FS, resolve(b.cwd, name), []byte(data), 0644)
}
func (b *Bash) GetCwd() string            { return b.cwd }
func (b *Bash) GetEnv() map[string]string { return cloneMap(b.env) }

func (b *Bash) execute(ctx context.Context, script, stdin string, cwd *string, env map[string]string, stdout, stderr io.Writer, depth int) int {
	chains, err := parse(script, env)
	if err != nil {
		fmt.Fprintf(stderr, "bash: syntax error: %v\n", err)
		return 2
	}
	count := 0
	last := 0
	for _, chain := range chains {
		if ctx.Err() != nil {
			return 124
		}
		if chain.Gate == "&&" && last != 0 {
			continue
		}
		if chain.Gate == "||" && last == 0 {
			continue
		}
		count += len(chain.Pipeline)
		if count > b.limits.MaxCommands {
			fmt.Fprintln(stderr, "bash: command count limit exceeded")
			return 126
		}
		last = b.runPipeline(ctx, chain.Pipeline, stdin, cwd, env, stdout, stderr, depth)
		env["?"] = fmt.Sprint(last)
	}
	return last
}

func (b *Bash) runPipeline(ctx context.Context, pipeline []simpleCommand, stdin string, cwd *string, env map[string]string, stdout, stderr io.Writer, depth int) int {
	input := []byte(stdin)
	code := 0
	for i, cmd := range pipeline {
		var stage bytes.Buffer
		target := io.Writer(&stage)
		if i == len(pipeline)-1 {
			target = stdout
		}
		in := io.Reader(bytes.NewReader(input))
		if cmd.Input != "" {
			data, e := gfs.ReadFile(b.FS, resolve(*cwd, cmd.Input))
			if e != nil {
				fmt.Fprintf(stderr, "bash: %s: %v\n", cmd.Input, e)
				return 1
			}
			in = bytes.NewReader(data)
		}
		var fileBuf bytes.Buffer
		if cmd.Output != "" {
			target = &fileBuf
		}
		code = b.runCommand(ctx, cmd, in, target, stderr, cwd, env, depth)
		if cmd.Output != "" {
			name := resolve(*cwd, cmd.Output)
			var e error
			if cmd.Append {
				e = gfs.AppendFile(b.FS, name, fileBuf.Bytes(), 0644)
			} else {
				e = gfs.WriteFile(b.FS, name, fileBuf.Bytes(), 0644)
			}
			if e != nil {
				fmt.Fprintf(stderr, "bash: %s: %v\n", cmd.Output, e)
				return 1
			}
		}
		if i < len(pipeline)-1 {
			input = append(input[:0], stage.Bytes()...)
		}
	}
	return code
}
func (b *Bash) runCommand(ctx context.Context, cmd simpleCommand, stdin io.Reader, stdout, stderr io.Writer, cwd *string, env map[string]string, depth int) int {
	for k, v := range cmd.Assign {
		env[k] = v
	}
	if len(cmd.Args) == 0 {
		return 0
	}
	name := cmd.Args[0]
	name = strings.TrimPrefix(name, "/bin/")
	name = strings.TrimPrefix(name, "/usr/bin/")
	if name == "cd" {
		return commandCD(ctx, cmd.Args[1:], &CommandContext{FS: b.FS, Cwd: cwd, Env: env, Stdin: stdin, Stdout: stdout, Stderr: stderr})
	}
	if name == "bash" || name == "sh" {
		if depth > 50 {
			fmt.Fprintln(stderr, "bash: maximum call depth exceeded")
			return 126
		}
		args := cmd.Args[1:]
		if len(args) >= 2 && args[0] == "-c" {
			data, _ := io.ReadAll(stdin)
			return b.execute(ctx, args[1], string(data), cwd, env, stdout, stderr, depth+1)
		}
		if len(args) > 0 {
			data, e := gfs.ReadFile(b.FS, resolve(*cwd, args[0]))
			if e != nil {
				fmt.Fprintln(stderr, e)
				return 1
			}
			return b.execute(ctx, string(data), "", cwd, env, stdout, stderr, depth+1)
		}
	}
	c, ok := b.commands[name]
	if !ok {
		fmt.Fprintf(stderr, "bash: %s: command not found\n", name)
		return 127
	}
	return c.Run(ctx, cmd.Args[1:], &CommandContext{FS: b.FS, Cwd: cwd, Env: env, Stdin: stdin, Stdout: stdout, Stderr: stderr})
}
