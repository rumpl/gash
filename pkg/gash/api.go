package gash

import (
	iofs "io/fs"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/rumpl/gash/internal/command"
	"github.com/rumpl/gash/internal/commands"
	gfs "github.com/rumpl/gash/pkg/fs"
	"github.com/rumpl/gash/pkg/network"
)

type Result struct {
	Stdout   string            `json:"stdout"`
	Stderr   string            `json:"stderr"`
	ExitCode int               `json:"exitCode"`
	Env      map[string]string `json:"env,omitempty"`
}
type Options struct {
	FS           iofs.FS
	Files        map[string]string
	Env          map[string]string
	Cwd          string
	Limits       Limits
	LimitProfile LimitProfile
	Commands     []Command
	Network      *network.Policy
	Now          func() time.Time
}
type ExecOptions struct {
	Env        map[string]string
	Cwd, Stdin string
	ReplaceEnv bool
	Args       []string
	ScriptName string
}
type (
	CommandFunc    = command.Func
	Command        = command.Command
	CommandContext = command.Context
)

type Bash struct {
	FS       iofs.FS
	env      map[string]string
	cwd      string
	commands map[string]Command
	limits   Limits
	now      func() time.Time
	mu       sync.Mutex
}

func New(options Options) (*Bash, error) {
	limits, err := resolveLimits(options.Limits, options.LimitProfile)
	if err != nil {
		return nil, err
	}
	filesystem := options.FS
	if filesystem == nil {
		filesystem = gfs.NewMemory(limits.MaxFileSystemBytes)
	}
	cwd := options.Cwd
	if cwd == "" {
		cwd = "/home/user"
	}
	cwd = canonicalDirectory(cwd)
	for _, dir := range []string{"/home/user", "/tmp", "/bin", "/usr/bin"} {
		_ = gfs.MkdirAll(filesystem, dir, 0o755)
	}
	for p, data := range options.Files {
		abs := resolve("/", p)
		_ = gfs.MkdirAll(filesystem, path.Dir(abs), 0o755)
		if err := gfs.WriteFile(filesystem, abs, []byte(data), 0o644); err != nil {
			return nil, err
		}
	}
	env := map[string]string{}
	env["PWD"] = cwd
	env["OLDPWD"] = cwd
	for k, v := range options.Env {
		env[k] = v
	}
	enforcePublicInternalEnv(env)
	if options.Now == nil {
		options.Now = time.Now
	}
	b := &Bash{FS: filesystem, env: env, cwd: cwd, commands: map[string]Command{}, limits: limits, now: options.Now}
	for _, c := range commands.BuiltinsWithNetwork(options.Network) {
		b.RegisterCommand(c)
	}
	for _, c := range options.Commands {
		b.RegisterCommand(c)
	}
	return b, nil
}

func canonicalDirectory(directory string) string {
	return path.Clean("/" + strings.TrimPrefix(directory, "/"))
}

func resolve(base, name string) string {
	if strings.HasPrefix(name, "/") {
		return path.Clean(name)
	}
	return path.Clean(path.Join(base, name))
}

func (b *Bash) RegisterCommand(c Command) {
	b.commands[c.Name] = c
}

func cloneMap(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
