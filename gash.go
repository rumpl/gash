// Package gash implements a bash-compatible shell over an io/fs filesystem.
package gash

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	iofs "io/fs"
	"os"
	"path"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	gfs "github.com/rumpl/gash/fs"
	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
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
	FS       iofs.FS
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
	Args       []string
}
type CommandFunc func(context.Context, []string, *CommandContext) int
type Command struct {
	Name string
	Run  CommandFunc
}
type CommandContext struct {
	FS             iofs.FS
	Cwd            *string
	Env            map[string]string
	Stdin          io.Reader
	Stdout, Stderr io.Writer
}

type Bash struct {
	FS       iofs.FS
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
	for _, dir := range []string{"/home/user", "/tmp", "/bin", "/usr/bin"} {
		_ = gfs.MkdirAll(filesystem, dir, 0755)
	}
	for p, data := range options.Files {
		abs := resolve("/", p)
		_ = gfs.MkdirAll(filesystem, path.Dir(abs), 0755)
		if err := gfs.WriteFile(filesystem, abs, []byte(data), 0644); err != nil {
			return nil, err
		}
	}
	env := map[string]string{"HOME": "/home/user", "PATH": "/usr/bin:/bin", "IFS": " \t\n", "OSTYPE": "linux-gnu", "MACHTYPE": "x86_64-pc-linux-gnu", "HOSTTYPE": "x86_64", "HOSTNAME": "localhost", "USER": "user", "UID": "1000", "EUID": "1000", "GID": "1000", "PWD": cwd, "OLDPWD": cwd}
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
func (b *Bash) RegisterCommand(c Command) { b.commands[c.Name] = c }
func cloneMap(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

type boundedBuffer struct {
	mu sync.Mutex
	bytes.Buffer
	limit, total int
	exceeded     bool
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	remaining := b.limit - b.total
	if remaining <= 0 {
		b.exceeded = true
		return len(p), nil
	}
	if len(p) > remaining {
		_, _ = b.Buffer.Write(p[:remaining])
		b.total += remaining
		b.exceeded = true
		return len(p), nil
	}
	n, _ := b.Buffer.Write(p)
	b.total += n
	return n, nil
}
func (b *boundedBuffer) String() string { b.mu.Lock(); defer b.mu.Unlock(); return b.Buffer.String() }

func (b *Bash) Exec(parent context.Context, script string, options ExecOptions) Result {
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
	ctx, cancel := context.WithTimeout(parent, b.limits.MaxExecutionTime)
	defer cancel()
	out := &boundedBuffer{limit: b.limits.MaxOutputBytes}
	errout := &boundedBuffer{limit: b.limits.MaxOutputBytes}
	code, finalEnv := b.execute(ctx, script, options.Stdin, cwd, env, options.Args, out, errout, 0)
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		fmt.Fprintln(errout, "bash: execution timed out")
		code = 124
	}
	if out.exceeded || errout.exceeded {
		fmt.Fprintln(errout, "bash: output size limit exceeded")
		code = 126
	}
	return Result{Stdout: out.String(), Stderr: errout.String(), ExitCode: code, Env: finalEnv}
}
func (b *Bash) execute(ctx context.Context, script, stdin, cwd string, env map[string]string, args []string, stdout, stderr io.Writer, depth int) (int, map[string]string) {
	parser := syntax.NewParser(syntax.Variant(syntax.LangBash))
	program, err := parser.Parse(strings.NewReader(script), "")
	if err != nil {
		fmt.Fprintf(stderr, "bash: %v\n", err)
		return 2, env
	}
	pairs := make([]string, 0, len(env))
	for k, v := range env {
		pairs = append(pairs, k+"="+v)
	}
	var commands atomic.Int64
	runner, err := interp.New(interp.Env(expand.ListEnviron(pairs...)), interp.Params(args...), interp.StdIO(strings.NewReader(stdin), stdout, stderr), interp.OpenHandler(b.openHandler), interp.ReadDirHandler2(b.readDirHandler), interp.StatHandler(b.statHandler), interp.ExecHandler(func(callCtx context.Context, argv []string) error {
		if commands.Add(1) > int64(b.limits.MaxCommands) {
			fmt.Fprintln(interp.HandlerCtx(callCtx).Stderr, "bash: command count limit exceeded")
			return interp.NewExitStatus(126)
		}
		return b.execCommand(callCtx, argv, depth)
	}))
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
func (b *Bash) execCommand(ctx context.Context, args []string, depth int) error {
	h := interp.HandlerCtx(ctx)
	if len(args) == 0 {
		return nil
	}
	name := strings.TrimPrefix(strings.TrimPrefix(args[0], "/bin/"), "/usr/bin/")
	env := map[string]string{}
	h.Env.Each(func(k string, v expand.Variable) bool {
		if v.IsSet() {
			env[k] = v.String()
		}
		return true
	})
	cwd := h.Dir
	if name == "bash" || name == "sh" {
		if depth >= 50 {
			fmt.Fprintln(h.Stderr, "bash: maximum call depth exceeded")
			return interp.NewExitStatus(126)
		}
		argv := args[1:]
		var script string
		var params []string
		if len(argv) >= 2 && argv[0] == "-c" {
			script = argv[1]
			if len(argv) > 2 {
				params = argv[2:]
			}
		} else if len(argv) > 0 {
			data, e := gfs.ReadFile(b.FS, resolve(cwd, argv[0]))
			if e != nil {
				fmt.Fprintf(h.Stderr, "%s: %v\n", name, e)
				return interp.NewExitStatus(1)
			}
			script = string(data)
			params = argv[1:]
		} else {
			data, _ := io.ReadAll(h.Stdin)
			script = string(data)
		}
		data, _ := io.ReadAll(h.Stdin)
		code, _ := b.execute(ctx, script, string(data), cwd, env, params, h.Stdout, h.Stderr, depth+1)
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

func (b *Bash) openHandler(ctx context.Context, name string, flag int, perm os.FileMode) (io.ReadWriteCloser, error) {
	name = handlerPath(ctx, name)
	if name == "/dev/null" {
		return &nullFile{}, nil
	}
	if flag&(os.O_WRONLY|os.O_RDWR) != 0 {
		if _, ok := b.FS.(gfs.WriteFileFS); !ok {
			return nil, gfs.ErrReadOnly
		}
	}
	var initial []byte
	if flag&os.O_WRONLY == 0 || flag&os.O_APPEND != 0 {
		data, err := gfs.ReadFile(b.FS, name)
		if err == nil {
			initial = data
		} else if flag&os.O_CREATE == 0 {
			return nil, err
		}
	}
	if flag&os.O_EXCL != 0 {
		if _, err := gfs.Stat(b.FS, name); err == nil {
			return nil, iofs.ErrExist
		}
	}
	vf := &virtualFile{fs: b.FS, name: name, perm: perm, write: flag&(os.O_WRONLY|os.O_RDWR) != 0, appendMode: flag&os.O_APPEND != 0, data: append([]byte(nil), initial...)}
	if flag&os.O_TRUNC != 0 {
		vf.data = nil
	}
	if vf.appendMode {
		vf.offset = len(vf.data)
	}
	return vf, nil
}
func (b *Bash) readDirHandler(ctx context.Context, name string) ([]iofs.DirEntry, error) {
	name = handlerPath(ctx, name)
	entries, err := gfs.ReadDir(b.FS, name)
	if err != nil {
		return nil, err
	}
	if name == "/bin" || name == "/usr/bin" {
		seen := map[string]bool{}
		for _, entry := range entries {
			seen[entry.Name()] = true
		}
		for command := range b.commands {
			if !seen[command] {
				entries = append(entries, syntheticEntry(command))
			}
		}
	}
	return entries, nil
}
func (b *Bash) statHandler(ctx context.Context, name string, follow bool) (iofs.FileInfo, error) {
	name = handlerPath(ctx, name)
	var info iofs.FileInfo
	var err error
	if follow {
		info, err = gfs.Stat(b.FS, name)
	} else {
		info, err = gfs.Lstat(b.FS, name)
	}
	if err != nil && (name == "/bin" || name == "/usr/bin") {
		return syntheticDirInfo(path.Base(name)), nil
	}
	if err != nil && (path.Dir(name) == "/bin" || path.Dir(name) == "/usr/bin") {
		base := path.Base(name)
		if _, ok := b.commands[base]; ok {
			return syntheticInfo(base), nil
		}
		if base == "bash" || base == "sh" {
			return syntheticInfo(base), nil
		}
	}
	return info, err
}

func handlerPath(ctx context.Context, name string) (result string) {
	if strings.HasPrefix(name, "/") {
		return path.Clean(name)
	}
	// Some interpreter-internal filesystem calls don't install HandlerContext.
	// They already pass cwd-resolved names; recover only guards that distinction.
	defer func() {
		if recover() != nil {
			result = path.Clean("/" + name)
		}
	}()
	return resolve(interp.HandlerCtx(ctx).Dir, name)
}

type syntheticInfo string

func (i syntheticInfo) Name() string      { return string(i) }
func (syntheticInfo) Size() int64         { return 0 }
func (syntheticInfo) Mode() iofs.FileMode { return 0755 }
func (syntheticInfo) ModTime() time.Time  { return time.Time{} }
func (syntheticInfo) IsDir() bool         { return false }
func (syntheticInfo) Sys() any            { return nil }

type syntheticDirInfo string

func (i syntheticDirInfo) Name() string      { return string(i) }
func (syntheticDirInfo) Size() int64         { return 0 }
func (syntheticDirInfo) Mode() iofs.FileMode { return iofs.ModeDir | 0755 }
func (syntheticDirInfo) ModTime() time.Time  { return time.Time{} }
func (syntheticDirInfo) IsDir() bool         { return true }
func (syntheticDirInfo) Sys() any            { return nil }

type syntheticEntry string

func (e syntheticEntry) Name() string                 { return string(e) }
func (syntheticEntry) IsDir() bool                    { return false }
func (syntheticEntry) Type() iofs.FileMode            { return 0 }
func (e syntheticEntry) Info() (iofs.FileInfo, error) { return syntheticInfo(e), nil }

type virtualFile struct {
	fs                iofs.FS
	name              string
	perm              os.FileMode
	data              []byte
	offset            int
	write, appendMode bool
}

func (f *virtualFile) Read(p []byte) (int, error) {
	if f.offset >= len(f.data) {
		return 0, io.EOF
	}
	n := copy(p, f.data[f.offset:])
	f.offset += n
	return n, nil
}
func (f *virtualFile) Write(p []byte) (int, error) {
	if !f.write {
		return 0, errors.New("file not open for writing")
	}
	end := f.offset + len(p)
	if end > len(f.data) {
		f.data = append(f.data, make([]byte, end-len(f.data))...)
	}
	copy(f.data[f.offset:end], p)
	f.offset = end
	return len(p), nil
}
func (f *virtualFile) Close() error {
	if !f.write {
		return nil
	}
	return gfs.WriteFile(f.fs, f.name, f.data, f.perm)
}

type nullFile struct{}

func (*nullFile) Read([]byte) (int, error)    { return 0, io.EOF }
func (*nullFile) Write(p []byte) (int, error) { return len(p), nil }
func (*nullFile) Close() error                { return nil }

func (b *Bash) ReadFile(name string) (string, error) {
	data, e := gfs.ReadFile(b.FS, resolve(b.cwd, name))
	return string(data), e
}
func (b *Bash) WriteFile(name, data string) error {
	return gfs.WriteFile(b.FS, resolve(b.cwd, name), []byte(data), 0644)
}
func (b *Bash) GetCwd() string            { return b.cwd }
func (b *Bash) GetEnv() map[string]string { return cloneMap(b.env) }
