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
| [`host-readonly`](host-readonly) | Exposing a host directory read-only with `os.DirFS` |
| [`host-rooted`](host-rooted) | Using the symlink-contained, writable `fs.Rooted` host filesystem |
| [`overlay`](overlay) | Writable in-memory changes over a read-only lower filesystem |
| [`mounts`](mounts) | Combining independent filesystems under virtual mount points |
| [`network`](network) | Opting into `curl` with an explicit origin/method policy |
| [`sqlite`](sqlite) | Creating and querying a SQLite database in the virtual filesystem |

The network example performs a real HTTPS request. All other examples run locally without network access. Host filesystem access is explicit: gash uses an isolated in-memory filesystem unless an `io/fs.FS` is supplied.
