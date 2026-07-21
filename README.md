# gash

A Go rewrite of [vercel-labs/just-bash](https://github.com/vercel-labs/just-bash): a bash-like interpreter with a bounded, in-memory virtual filesystem. It is intended for agents and applications that need useful shell workflows without exposing the host filesystem or launching host processes.

> **Status:** early development. The core execution and filesystem model is implemented, but this is not yet command-for-command compatible with just-bash.

## Install

```sh
go get github.com/rumpl/gash
# or install the CLI
go install github.com/rumpl/gash/cmd/gash@latest
```

## Library usage

```go
package main

import (
    "context"
    "fmt"

    "github.com/rumpl/gash"
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
printf 'echo "$NAME"\n' | gash -e NAME=world
gash --json -c 'false'
gash script.sh
```

The CLI starts with a fresh in-memory filesystem. A script filename is read by the CLI as input; commands inside the script cannot access host files.

## Supported behavior

Shell syntax:

- single and double quotes, backslash escaping, and comments
- `$VAR`, `${VAR}`, `${VAR:-default}`, and `$?` expansion
- pipelines (`|`), lists (`;` and newlines), `&&`, and `||`
- input, truncate, and append redirections (`<`, `>`, and `>>`)
- temporary execution environment/cwd and command assignments
- nested `bash -c`, `sh -c`, and virtual shell script execution

Built-in commands:

`base64`, `basename`, `bash`, `cat`, `cd`, `clear`, `cp`, `dirname`, `echo`, `env`, `false`, `grep`, `head`, `hostname`, `ln -s`, `ls`, `md5sum`, `mkdir`, `mv`, `printf`, `printenv`, `pwd`, `readlink`, `rm`, `rmdir`, `seq`, `sha1sum`, `sha256sum`, `sh`, `sleep`, `sort`, `tail`, `tee`, `touch`, `true`, `uniq`, `wc`, and `whoami`.

Not yet implemented include functions, loops, conditionals (`if`), command substitution, arithmetic, glob expansion, heredocs, jobs, mount/overlay filesystems, and optional network/language runtimes.

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

## Development

```sh
make test       # tests, race detector, and vet
make build      # build ./bin/gash
```

## License

Apache-2.0. See [NOTICE](NOTICE) for upstream inspiration.
