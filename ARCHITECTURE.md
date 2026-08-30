# Architecture

Gash is organized around capability boundaries so the shell can be embedded in production services without accidentally acquiring host access.

## Package layout

```text
cmd/gash/              CLI composition root
pkg/gash/              public shell API and execution orchestration
  api.go               constructors, public types, registration
  exec.go              parse/run lifecycle and command dispatch
  fs_handlers.go       mvdan interpreter to io/fs adapter
  limits.go            profiles and shared execution accounting
  output.go            concurrency-safe bounded output sink
pkg/fs/                public filesystem implementations
  capabilities.go      io/fs helpers and optional write capabilities
  memory.go            bounded in-memory filesystem
  mount.go             mount routing and read namespace
  mount_write.go       mutation and cross-mount operations
  database.go          SQL-table-backed filesystem over the DB interface
  sqlite.go            SQLite backend (modernc.org/sqlite, database/sql)
  turso.go             Turso/libSQL backend over the Hrana v2 HTTP protocol
  copy.go              copy mounts: copying a source tree into a filesystem
internal/command/      dependency-neutral command contract
internal/commandutil/  shared command helpers
internal/commands/     registry and command-family packages
  files/               file command package; one command per source file
                       with an adjacent `<command>_test.go`
  text/                text command package; one command per source file
                       with an adjacent `<command>_test.go`
  utilities/           command-execution utilities such as `xargs`
```

Tests live beside their packages. Commands are implemented one per source file with an adjacent command test. `public_test.go` compiles as an external consumer so internal types cannot accidentally leak through the public API.

All Go source is formatted with `gofumpt`; CI runs tests, race detection, vet, and a formatting check.

## Dependency direction

```text
cmd/gash
   |
   v
pkg/gash  ---> internal/commands ---> internal/command
   |                 |
   +-----------------+----> pkg/fs

pkg/fs ---> io/fs
```

`pkg/fs` has no shell dependency. Built-in commands know only the small command context and filesystem capabilities. The public shell package is the composition root.

## Filesystem boundary

The base contract is `io/fs.FS`, which is intentionally read-only and works with standard implementations. Mutations are separate optional interfaces (`WriteFileFS`, `MkdirFS`, `RenameFS`, and others). This provides least authority and avoids forcing every backend to pretend it is writable.

Shell paths are absolute Unix paths. They are converted to root-relative, `fs.ValidPath`-compatible names at the boundary. The interpreter receives custom open, stat, and directory handlers and never receives the host defaults.

Database-backed filesystems keep that contract: `pkg/fs.Database` stores paths and nodes as rows and reaches its backend through the two-method `DB` interface, so the SQLite driver and the Turso HTTP client are interchangeable and neither is visible to commands. Copy mounts import a source tree once through the same capability helpers, so no shell command can reach the source filesystem afterwards.

## Execution boundary

Each public `Exec` creates:

1. an isolated environment and cwd;
2. one context deadline;
3. one execution scope shared by nested shells;
4. one aggregate output budget shared by stdout and stderr;
5. a Bash AST and a fresh interpreter runner.

The filesystem belongs to `Bash` and persists across executions. Shell variables, functions, options, and cwd do not.

Built-ins are selected from an explicit registry. Unknown commands return 127; no fallback invokes `os/exec`.

## Extension boundary

Custom commands receive `CommandContext`, containing only the configured filesystem, virtual cwd/environment, standard streams, and cancellation context. New host capabilities must be explicit fields or interfaces rather than package globals.

The current custom-command boundary is trusted Go code. A revocable restricted extension scope remains parity work and is tracked in `PORTING.md`.
