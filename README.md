# gash

A Go rewrite of [vercel-labs/just-bash](https://github.com/vercel-labs/just-bash): a Bash interpreter with a capability-based virtual filesystem. It is intended for agents and applications that need useful shell workflows without exposing the host filesystem or launching host processes.

> **Status:** active practical rewrite work. Bash parsing/execution uses a full AST interpreter, and common search/text workflows now include in-process `awk`, `sed`, `find`, `xargs`, `diff`, `expr`, and `rg` implementations with documented parity gaps. Gash uses pinned just-bash behavior as guidance while documenting intentional deferrals. See the authoritative [porting status and backlog](PORTING.md).

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
- pipelines, lists, subshells, `&&`, `||`, isolated asynchronous background jobs, `$!`, `wait`, and `jobs`
- functions, `if`, `case`, C-style and word `for` loops, `while`, and `until`
- test expressions, shell options, arrays, and positional parameters
- redirections, heredocs, and process substitution supported by the interpreter
- nested `bash -c`, `sh -c`, and virtual shell script execution
- virtual `HUP`, `INT`, `QUIT`, and `TERM` traps through a host-isolated `kill` command

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

`awk`, `alias`, `base64`, `basename`, `bash`, `cat`, `cd`, `chmod`, `cksum`, `clear`, `column`, `comm`, `cp`, `curl` (opt-in network only), `cut`, `date`, `diff`, `dirname`, `du`, `echo`, `egrep`, `env`, `expand`, `expr`, `factor`, `false`, `fgrep`, `file`, `find`, `fold`, `grep`, `gunzip`, `gzip`, `hash`, `head`, `help`, `history`, `hostname`, `html-to-markdown`, `id`, `join`, `jq`, `ln`, `ls`, `md5sum`, `mkdir`, `mktemp`, `mv`, `nl`, `od`, `paste`, `printf`, `printenv`, `pwd`, `readlink`, `realpath`, `rev`, `rg`, `rm`, `rmdir`, `sed`, `seq`, `sha1sum`, `sha256sum`, `sha512sum`, `sh`, `shuf`, `sleep`, `sort`, `split`, `sqlite3`, `stat`, `strings`, `tac`, `tail`, `tar`, `tee`, `time`, `timeout`, `touch`, `tr`, `tree`, `true`, `umask`, `unalias`, `unexpand`, `uniq`, `unlink`, `uuidgen`, `wc`, `which`, `whoami`, `xan`, `xargs`, `yes`, `yq`, and `zcat`.

`hash` is compatibility glue backed by an always-empty virtual table: `hash -r` is a no-op, names resolve only against shell and gash commands, and host `PATH` entries are never resolved, cached, or reported.
Command flags and edge cases are still being ported from the upstream command test suites for selected tasks. Transform support and optional language runtimes remain intentional deferrals unless explicitly enabled by a future task. `curl`/network support is opt-in: library users must pass `Options.Network`, and the CLI must pass `--network-allow scheme://host[:port][/path]`; allowed redirects are rechecked and private/loopback/link-local DNS targets are blocked by default. A mountable `io/fs` implementation is available as `fs.Mountable`.

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

`fs.Overlay` keeps a read-only lower filesystem intact while directing every mutation to the writable upper filesystem. Modifying an entry that exists only in the lower layer copies it up first, so appends, permission changes, timestamp changes, hard links, and renames observe the lower content. Removing a lower entry records a whiteout marker in the upper layer, so the deletion survives in a persisted upper directory and the lower entry stays hidden; removing a directory makes it opaque, so a directory recreated at the same path starts empty. Upper entries whose base name starts with `.wh.` are reserved for those markers: they are never listed and cannot be created or read through the overlay. Copy-up preserves the lower permission bits and adds owner access so a host-backed upper layer stays writable.

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

## Examples

Runnable programs for in-memory, seeded, custom-command, Docker Agent, host filesystem, overlay, mount, network-policy, security-boundary, and SQLite setups are available in [`examples/`](examples/).

```sh
go run ./examples/basic
go run ./examples/host-readonly -- .
(cd examples/docker-agent && go run . "Inspect the project with the shell tool")
go run ./examples/security
go run ./examples/sqlite
```

## WebAssembly

Gash can run in a browser through the `js/wasm` target. The JavaScript API exposes a persistent in-memory shell as `gash.exec(script, options)`, returning a Promise with `stdout`, `stderr`, `exitCode`, and `env`. The browser build excludes `sqlite3`, whose Go dependency does not support `js/wasm`; host filesystem and network capabilities are not enabled. Go’s browser port also lacks `os.Pipe`, so pipelines, heredocs, command substitution, and non-empty `stdin` are not available in this target; scripts using ordinary commands, control flow, and virtual-file redirections run normally.

Build and open the included demo:

```sh
make serve-wasm
# visit http://localhost:8080
```

`options` may contain `cwd`, `env`, `replaceEnv`, `args`, and `scriptName`. (`stdin` is reserved for parity with the Go API but must currently be empty.) Call `gash.reset()` to replace the persistent virtual filesystem with a fresh one.

## Development

```sh
make test       # tests, race detector, and vet
make build      # build ./bin/gash
make wasm       # build web/gash.wasm and copy Go's WebAssembly runtime
```

## License

Apache-2.0. See [NOTICE](NOTICE) for upstream inspiration.
