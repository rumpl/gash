# Gash examples

Each subdirectory is an independent executable example. Run one from the repository root with:

```sh
go run ./examples/basic
```

## Setups

| Example | Setup |
| --- | --- |
| [`basic`](basic) | Default bounded in-memory filesystem and a shell pipeline |
| [`seeded`](seeded) | Initial files, environment variables, execution arguments, and hardened limits |
| [`custom-command`](custom-command) | Registering a trusted Go command and using it in a pipeline |
| [`docker-agent`](docker-agent) | A Docker Agent whose capability-scoped `shell` tool is gash |
| [`host-readonly`](host-readonly) | Exposing a host directory read-only with `os.DirFS` |
| [`host-rooted`](host-rooted) | Using the symlink-contained, writable `fs.Rooted` host filesystem |
| [`overlay`](overlay) | Writable in-memory changes over a read-only lower filesystem |
| [`persistent-overlay`](persistent-overlay) | Saving the writable diff to disk and reusing it on later runs |
| [`mounts`](mounts) | Combining independent filesystems under virtual mount points |
| [`sqlite-fs`](sqlite-fs) | Persisting the whole filesystem in a SQLite database and copy-mounting a host directory |
| [`turso-fs`](turso-fs) | Sharing one filesystem across machines through a Turso (libSQL) database |
| [`network`](network) | Opting into `curl` with an explicit origin/method policy |
| [`security`](security) | Read-only filesystem, network allowlist, host-command denial, and hardened limits |
| [`sqlite`](sqlite) | Creating and querying a SQLite database in the virtual filesystem |

The network example performs a real HTTPS request. The Docker Agent example calls the configured model provider and may use only explicitly allowed network origins from its gash tool. The Turso example needs a reachable Turso database (`TURSO_DATABASE_URL` and `TURSO_AUTH_TOKEN`, or a local `turso dev` server). All other examples run locally without network access. The security example uses a deterministic mock HTTP transport after applying the real URL policy. Host filesystem access is explicit: gash uses an isolated in-memory filesystem unless an `io/fs.FS` is supplied.

## Database-backed filesystems

The sqlite-fs example keeps the entire virtual filesystem in a SQLite file and copies a host directory into it. Copy mounts are not routed: the data is owned by the database afterwards, so the host directory may be deleted without the shell losing anything.

```sh
# Copies the current directory to /project on the first run only, then
# records one more run in the database each time.
go run ./examples/sqlite-fs
go run ./examples/sqlite-fs

# Choose the database and the copied directory, and re-copy the source.
go run ./examples/sqlite-fs -db ./state.db -mount ./project -replace
```

## Persistent overlay

The persistent-overlay example treats the upper directory as the saved diff. The lower directory is read through `os.DirFS`, while every new or changed upper-layer file is stored under a separate `fs.Rooted` directory:

```sh
# Uses the current directory as the lower layer and a user-cache directory as
# the persistent upper layer. Running it repeatedly increments the run count.
go run ./examples/persistent-overlay
go run ./examples/persistent-overlay

# Choose the locations explicitly.
go run ./examples/persistent-overlay \
  -lower ./project-template \
  -upper ./saved-overlay

# Clear the upper layer before this run.
go run ./examples/persistent-overlay -upper ./saved-overlay -reset
```

The upper directory is reusable filesystem state, not a textual patch. Keep it outside the lower directory to avoid exposing the saved diff through both layers. The current overlay implementation does not use whiteouts, so deleting a file that exists only in the lower layer does not persistently hide it.
