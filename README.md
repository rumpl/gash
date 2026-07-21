# gash

A Go rewrite of [vercel-labs/just-bash](https://github.com/vercel-labs/just-bash): a Bash interpreter with a capability-based virtual filesystem. It is intended for agents and applications that need useful shell workflows without exposing the host filesystem or launching host processes.

> **Status:** active practical rewrite work. Bash parsing/execution now uses a full AST interpreter, and near-term work prioritizes missing search/text commands such as `fgrep`, `sed`, `find`, `xargs`, `diff`, `expr`, `rg`, and `awk`. Gash uses pinned just-bash behavior as guidance for selected tasks while documenting intentional deferrals. See [remaining work](REMAINING_WORK.md) and [porting notes](PORTING.md).

## Install

```sh
go get github.com/rumpl/gash/pkg/gash
# or install the CLI
go install github.com/rumpl/gash/cmd/gash@latest
```

## Library usage

```go
package main

import (
    "context"
    "fmt"

    "github.com/rumpl/gash/pkg/gash"
)

func main() {
    shell, err := gash.New(gash.Options{
        Files: map[string]string{"/data/names.txt": "Ada\nGrace\n"},
        Env:   map[string]string{"GREETING": "hello"},
    })
    if err != nil {
        panic(err)
    }

    result := shell.Exec(context.Background(),
        `cat /data/names.txt | grep Grace && echo "$GREETING"`,
        gash.ExecOptions{},
    )
    fmt.Print(result.Stdout)
}
```

Each `Exec` receives isolated environment and working-directory state. The virtual filesystem belongs to the `Bash` instance and persists between calls.

### Custom commands

```go
upper := gash.Command{
    Name: "upper",
    Run: func(ctx context.Context, args []string, c *gash.CommandContext) int {
        data, _ := io.ReadAll(c.Stdin)
        fmt.Fprint(c.Stdout, strings.ToUpper(string(data)))
        return 0
    },
}

shell, _ := gash.New(gash.Options{Commands: []gash.Command{upper}})
result := shell.Exec(context.Background(), `echo hello | upper`, gash.ExecOptions{})
```

Custom commands receive only explicit capabilities: the virtual filesystem, virtual cwd/environment, standard streams, and execution context.

## CLI

```sh
gash -c 'echo hello | grep hell'
gash --root . -c 'ls; cat README.md'
printf 'echo "$NAME"\n' | gash -e NAME=world
gash --json -c 'false'
gash script.sh
```

The CLI starts with a fresh in-memory filesystem by default. Pass `--root DIR` to expose a host directory read-only as `/`; the virtual working directory then defaults to `/`. A script filename is read by the CLI as input but does not, by itself, expose the script's directory.

## Supported behavior

Shell syntax is parsed as Bash into an AST by `mvdan.cc/sh/v3`. It includes:

- quoting, parameter expansion, command substitution, and arithmetic expansion
- pipelines, lists, subshells, background statements, `&&`, and `||`
- functions, `if`, `case`, C-style and word `for` loops, `while`, and `until`
- test expressions, shell options, arrays, and positional parameters
- redirections, heredocs, and process substitution supported by the interpreter
- nested `bash -c`, `sh -c`, and virtual shell script execution

Built-in commands are summarized in the product-oriented
[feature support manifest](docs/status/feature-support.json), which is seeded
from the pinned just-bash command registry and checked by registry tests. Manifest
statuses mean:

- `core`: implemented as dependable baseline shell/product surface;
- `useful`: implemented as a practical in-process subset for common workflows;
- `partial`: present, but intentionally limited enough to check help/tests/notes;
- `optional`: only appropriate behind an explicit future capability or runtime policy;
- `deferred`: tracked from upstream but not implemented yet;
- `unsupported`: intentionally outside the current gash product surface.

Current registered built-ins include:

`awk`, `base64`, `basename`, `bash`, `cat`, `cd`, `chmod`, `clear`, `column`, `comm`, `cp`, `cut`, `diff`, `dirname`, `du`, `echo`, `egrep`, `env`, `expand`, `expr`, `false`, `fgrep`, `file`, `fold`, `grep`, `head`, `hostname`, `join`, `ln`, `ls`, `md5sum`, `mkdir`, `mv`, `nl`, `od`, `paste`, `printf`, `printenv`, `pwd`, `readlink`, `rev`, `rg`, `rm`, `rmdir`, `sed`, `seq`, `sha1sum`, `sha256sum`, `sh`, `sleep`, `sort`, `split`, `stat`, `strings`, `tac`, `tail`, `tee`, `touch`, `tr`, `tree`, `true`, `unexpand`, `uniq`, `wc`, and `whoami`.

Command flags and edge cases are still being ported from the upstream command test suites for selected tasks. Overlay filesystems, transforms, network commands, data runtimes, and optional language runtimes are intentional deferrals unless explicitly enabled by a future task. A mountable `io/fs` implementation is available as `fs.Mountable`.

## Filesystems

`Options.FS` accepts the standard library's minimal `io/fs.FS` interface. Standard read-only implementations work directly:

```go
shell, _ := gash.New(gash.Options{
    FS: fstest.MapFS{"data/input.txt": {Data: []byte("hello\n")}},
    Cwd: "/",
})
```

Writable implementations opt into small capability interfaces from `github.com/rumpl/gash/pkg/fs`, such as `WriteFileFS`, `MkdirFS`, `RemoveFS`, `RenameFS`, and `SymlinkFS`. Commands return a read-only error when the supplied implementation lacks a required capability. The bundled `fs.Memory` implements the complete capability set and the standard `fs.FS`, `fs.ReadFileFS`, `fs.ReadDirFS`, and `fs.StatFS` interfaces.

Virtual absolute shell paths are translated to valid root-relative `io/fs` paths only at the filesystem boundary.

The CLI's `--root DIR` option exposes a host directory as `/` through `fs.Rooted`. `fs.Rooted` resolves symlinks under the configured root and rejects lexical traversal or symlink escapes outside that directory. It implements write capabilities for gash commands, so scripts can mutate files inside `DIR`; use a read-only `Options.FS` implementation when mutation must be disallowed.

## Limits and security

Defaults are deliberately bounded:

- 20,000 commands per execution
- 32 MiB output
- 30 second execution deadline
- 1 GiB retained virtual file data
- shell recursion depth of 50
- symbolic-link traversal depth of 40

Override them with `gash.Options{Limits: gash.Limits{...}}`. A zero field selects its default.

Gash does not invoke `os/exec`, and its default filesystem does not touch disk. It is an application-level capability boundary, not a kernel or VM sandbox. Custom commands are trusted Go code and can use any host capability they import; do not register untrusted implementations.

## Architecture

The public libraries are `pkg/gash` and `pkg/fs`; implementation-only command contracts and built-ins live under `internal/`. See [ARCHITECTURE.md](ARCHITECTURE.md) for package responsibilities, dependency direction, and security boundaries.

## Development

```sh
make test       # tests, race detector, and vet
make build      # build ./bin/gash
```

## License

Apache-2.0. See [NOTICE](NOTICE) for upstream inspiration.
