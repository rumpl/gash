# just-bash porting status

This document is the human-readable source of truth for the Go rewrite of
[`vercel-labs/just-bash`](https://github.com/vercel-labs/just-bash). The
machine-readable command inventory is
[`docs/status/feature-support.json`](docs/status/feature-support.json), and must
remain consistent with this document.

Pinned upstream reference:

- repository: `vercel-labs/just-bash`
- commit: [`2b316eb26b3f3e832e2cf3994d4fef160d5eb8e4`](https://github.com/vercel-labs/just-bash/tree/2b316eb26b3f3e832e2cf3994d4fef160d5eb8e4)
- command registry: [`packages/just-bash/src/commands/registry.ts`](https://github.com/vercel-labs/just-bash/blob/2b316eb26b3f3e832e2cf3994d4fef160d5eb8e4/packages/just-bash/src/commands/registry.ts)

## How to read the status

A registered command is not automatically parity-complete. Statuses mean:

- **core**: dependable for common gash workflows;
- **useful**: a practical in-process implementation with known flag or edge-case gaps;
- **partial**: present, but compatibility limitations are significant enough that callers should consult help and tests;
- **optional**: available only with an explicit capability, or planned as an explicit opt-in runtime;
- **deferred**: represented by upstream but not implemented;
- **unsupported**: intentionally excluded.

None of these labels alone means every upstream test passes. Exact parity requires
ported upstream tests for flags, output, diagnostics, status codes, limits,
encoding, filesystem semantics, cancellation, and security behavior.

Current command inventory:

| Status      | Count |
| ----------- | ----: |
| Core        |    22 |
| Useful      |    35 |
| Partial     |    27 |
| Optional    |     5 |
| Deferred    |     0 |
| Unsupported |     0 |

All ordinary command names in the pinned upstream registry now have a gash
implementation or shell-native representation. The remaining command-name gaps
are the optional Python and JavaScript runtimes. Behavioral parity remains
substantial work.

## Implemented foundations

### Shell and execution

- [x] Parse Bash syntax into an AST with `mvdan.cc/sh/v3`.
- [x] Execute quoting, parameter expansion, command substitution, arithmetic,
      pipelines, lists, logical operators, functions, conditionals, loops,
      subshells, arrays, heredocs, and redirections in process.
- [x] Execute nested `bash` and `sh` scripts without host processes.
- [x] Isolate every pipeline component by default, matching Bash without
      `lastpipe`, and run EXIT traps installed in explicit subshells.
- [x] Treat redirection-open failures as command failures rather than fatal
      interpreter errors when `errexit` is disabled.
- [x] Support `set -C`/`set -o noclobber`, their disabling forms, and `>|`
      forced-clobber redirection without exposing interpreter panics.
- [x] Route argument-bearing `wait` calls to virtual jobs without exposing the
      upstream interpreter panic, and contain unexpected synchronous interpreter
      panics at the embedding boundary.
- [x] Run background `&` statements asynchronously in isolated interpreter
      environments, with virtual PIDs, `$!`, `wait`, status propagation, and
      deterministic `jobs` output.
- [x] Isolate shell variables, functions, options, arguments, cwd, and environment
      between top-level `Exec` calls.
- [x] Pass only exported variables to commands and nested shells; keep shell IFS
      unexported by default.
- [x] Support `export -p`, `declare -p`, and `readonly -p` with deterministic,
      shell-reusable declaration output.
- [x] Evaluate simple `test -r`, `[ -r ... ]`, and `[[ -r ... ]]` predicates
      through the injected filesystem's actual read capability, including
      least-authority read-only wrappers.
- [x] Persist the configured virtual filesystem between calls.
- [x] Return 127 for unknown commands; never fall back to `os/exec`.
- [x] Resolve `command -v` from shell built-ins and the capability-scoped gash
      registry rather than consulting the host PATH.
- [x] Canonicalize configured and per-execution working directories inside `/`,
      and reject relative, missing, or non-directory cwd values before execution.
- [x] Default `HOME` to the always-representable virtual root `/`.
- [x] Reject process substitution because the interpreter implementation would
      otherwise require a host-backed mechanism.
- [x] Reject unsupported `coproc`, `printf -v`, and `PIPESTATUS` usage before
      execution with compatibility diagnostics instead of internal failures or
      silently incorrect values.
- [x] Replace `$$` and `PPID` with virtual values.
- [x] Support virtual `HUP`, `INT`, `QUIT`, and `TERM` traps triggered by a
      capability-scoped `kill` command; no host process is signaled.
- [x] Share source, input, command-count, nesting, timeout, and aggregate-output
      budgets across an execution.
- [x] Provide normal and hardened limit profiles.
- [x] Support deterministic time injection with `Options.Now`.

### Filesystems

- [x] Use standard `io/fs.FS` as the public base contract.
- [x] Discover mutation support through granular optional capabilities.
- [x] Provide a bounded writable in-memory filesystem.
- [x] Support files, directories, symlinks, hard links, chmod, timestamps,
      rename, and recursive removal.
- [x] Provide a mountable filesystem with virtual parent directories and
      cross-mount copy/move support.
- [x] Provide a writable upper/read-only lower overlay filesystem.
- [x] Provide a writable rooted host filesystem that rejects lexical traversal
      and resolved symlink escapes.
- [x] Accept arbitrary standard read-only `io/fs` implementations.
- [x] Fail writes when the supplied filesystem lacks mutation capabilities.
- [x] Route shell redirections through the virtual filesystem.
- [x] Provide `/dev/null` through virtual command and redirection I/O without a
      host device dependency.
- [x] Provide least-authority read-only views that strip mutation capabilities.

### Security boundaries

- [x] Do not invoke host executables.
- [x] Do not expose the host filesystem by default.
- [x] Keep network access disabled unless an explicit policy is supplied.
- [x] Validate network scheme, host, port, path, method, redirects, request size,
      response size, timeout, and private-address policy.
- [x] Keep archive and SQLite operations inside virtual filesystem capabilities.
- [x] Reject archive traversal and unsafe archive links.
- [x] Disable SQLite extension loading and host-path escape mechanisms.
- [x] Document that custom Go commands are trusted native code rather than
      sandboxed untrusted code.

### Public API and CLI

- [x] Public packages are `pkg/gash`, `pkg/fs`, and `pkg/network`.
- [x] Support initial files, environment, cwd, arguments, custom commands,
      limits, deterministic time, filesystem injection, and network policy.
- [x] Support CLI command strings, script files, stdin, script arguments,
      environment overrides, JSON output, virtual cwd, rooted host access, and
      opt-in network origins.
- [x] Provide runnable examples for memory, seeded state, custom commands,
      Docker Agent integration, read-only and writable host filesystems,
      overlays, persistent overlays, mounts, network policy, SQLite, and
      security boundaries.

## Command status

### Core

These commands are expected to be dependable for common workflows, but their
status does not certify exhaustive upstream edge-case parity:

`base64`, `basename`, `bash`, `cat`, `cd`, `clear`, `dirname`, `echo`, `env`,
`false`, `hostname`, `md5sum`, `printenv`, `printf`, `pwd`, `seq`, `sh`,
`sha1sum`, `sha256sum`, `sleep`, `true`, `whoami`.

### Useful

These commands have practical in-process implementations with known unported
flags or edge cases:

`chmod`, `column`, `comm`, `cp`, `cut`, `du`, `expand`, `file`, `fold`, `head`,
`join`, `ln`, `ls`, `mkdir`, `mv`, `nl`, `od`, `paste`, `readlink`, `rev`, `rm`,
`rmdir`, `sort`, `split`, `stat`, `strings`, `tac`, `tail`, `tee`, `touch`, `tr`,
`tree`, `unexpand`, `uniq`, `wc`, `yes`.

### Partial

These commands work for documented subsets and remain priority parity areas:

- `awk` — in-process AWK subset; complete language/runtime behavior remains.
- `date` — common formats, UTC, date parsing, and deterministic time are present.
- `diff` — practical comparison and unified output; larger hunk formatting may differ.
- `grep`, `egrep`, `fgrep` — practical regex/fixed-string behavior; flags and
  binary/locale cases remain.
- `expr` — practical expression subset.
- `find` — practical predicates/actions and capability-scoped `-exec ... ;`/`+`
  dispatch; full expression truth propagation and traversal behavior remains.
- `rg` — bounded virtual-filesystem search; not complete ripgrep compatibility.
- `sed` — sandboxed stream-editor subset; shell-execution commands are rejected.
- `xargs` — whitespace/custom/NUL delimiters, replacement, batching (including
  attached forms such as `-n1`), parallel execution, verbose output, UTF-8, and
  safe command dispatch are present;
  complete GNU quoting and additional flags remain.
- `help` — lists registered commands and catalog help; full interactive Bash help differs.
- `history` — registered, but persistent interactive history is not implemented.
- `alias`, `unalias` — shell-native and scoped to one `Exec`.
- `which` — reports virtual gash commands, never host `PATH` executables.
- `time` — wall time is reported; host CPU accounting is intentionally absent.
- `timeout` — context cancellation works; additional GNU flags remain.
- `gzip`, `gunzip`, `zcat`, `tar` — in-process gzip and ustar/pax support with
  traversal and size protections; advanced flags, metadata cases, and codecs remain.
- `jq` — practical gojq-backed subset; modules, file imports, uncommon flags,
  and exact diagnostics remain.
- `yq` — YAML/gojq subset; round-trip comments/styles, in-place updates,
  additional formats, and complete CLI behavior remain.
- `xan` — practical CSV select/filter/sort/map/aggregate/group/view subset;
  complete expressions, reshape/join/statistics, and styling remain.
- `html-to-markdown` — common HTML structures are supported; exact whitespace,
  plugins, and uncommon constructs remain.
- `sqlite3` — embedded CGO-free SQLite with virtual image load/write-back,
  per-file locking, common modes/options, cancellation, and limits; exact CLI,
  formatter, error-recovery, locking, and upstream fixture parity remain.

### Optional

- [x] `curl` — implemented only when `Options.Network` or CLI
      `--network-allow` provides an explicit policy.
- [ ] `python3` / `python` — no embedded Python runtime yet.
- [ ] `js-exec` / `node` — no embedded JavaScript runtime yet.

`curl` still lacks proxy support, netrc, client certificates, complete cookie-jar
persistence, progress meters, the complete write-out variable set, and some TLS
and redirect/header edge cases.

## Remaining shell parity work

Gash uses `mvdan.cc/sh/v3`, while upstream just-bash has its own parser and
interpreter. The following require explicit differential testing rather than
assumption:

- [ ] Expansion ordering across quoting, arrays, `$@`, `$*`, `IFS`, globs,
      arithmetic, command substitution, and advanced parameter operators.
- [ ] Indexed and associative-array edge cases.
- [ ] `set`, `shopt`, shell-option defaults, and special-variable behavior.
- [ ] Alias expansion order and function/local-variable call-frame behavior.
- [ ] `[[ ... ]]`, regex, arithmetic-test, and pattern-matching differences.
- [ ] File-descriptor duplication/closing and uncommon redirection ordering.
- [x] Support dynamic, capability-safe `RANDOM` reads in the Bash range
      `0..32767` without exporting the special variable.
- [ ] Support Bash-compatible `RANDOM` assignment/seeding semantics.
- [ ] Implement virtual `umask`, `PIPESTATUS`, and Bash printf time formatting.
- [ ] Complete function/alias-aware `command` and `type` discovery.
- [ ] Complete background inheritance for shell functions, positional
      parameters, and non-default shell options.
- [ ] Complete background-job trap, signal, and cancellation semantics beyond
      virtual `HUP`/`INT`/`QUIT`/`TERM` delivery, including external
      context-to-trap delivery and exact default signal termination.
- [ ] Exact nested `bash`/`sh`, script-file, and executable virtual-script behavior.
- [ ] Exact command-not-found, not-executable, and directory-as-command diagnostics.
- [ ] Audit and virtualize `BASHPID` and any remaining host-derived shell metadata.
- [ ] Decide whether isolated process substitution can be implemented without
      introducing host process/filesystem capabilities.

## Remaining execution and limit work

Current public limits cover source bytes, execution depth, call depth, command
count, aggregate input, aggregate output, execution time, and filesystem bytes.
Upstream has finer-grained accounting still to port where applicable:

- [ ] Maximum string length and retained/live bytes.
- [ ] Array elements, arguments, and environment size.
- [ ] Expansion, regex, search, and loop work.
- [ ] Archive entry count and expanded-byte accounting integrated with execution scope.
- [ ] SQLite statement work and memory accounting integrated with execution scope.
- [ ] Nested command input/output accounting at every command boundary.
- [ ] Text-versus-byte output-kind propagation through pipelines and redirects.
- [ ] Exact UTF-8, invalid UTF-8, Latin-1, NUL-byte, and final-newline behavior.
- [ ] Immediate cancellation and cleanup across parallel commands, nested shells,
      SQLite, networking, and archive operations.
- [ ] Goroutine, lock, file-handle, and retained-memory leak tests.

## Remaining filesystem work

- [ ] Add true overlay copy-up when modifying a file that exists only in the lower layer.
- [ ] Add overlay whiteouts so deleting a lower-layer entry remains hidden.
- [ ] Define and test persistent overlay diff serialization/versioning if a portable
      artifact is needed beyond a persistent upper directory.
- [ ] Complete stable inode/device/link-count semantics needed by `stat`, hard links,
      archives, and SQLite.
- [ ] Audit mode, ownership, timestamp, and metadata preservation command by command.
- [ ] Add lazy initial files equivalent to upstream.
- [ ] Define atomicity guarantees for write, append, rename, link, and cross-mount copy.
- [ ] Expand nested-mount, shadowing, symlink, hard-link, and cross-device tests.
- [ ] Harden `Rooted` against symlink-swap/TOCTOU races for hostile concurrent host changes.
- [ ] Add shared conformance tests for every writable filesystem implementation.

## Remaining command parity work

Every command touched for parity must be compared with its upstream source and
all associated tests. Common unfinished areas are:

- [ ] Short, long, combined, repeated, attached-value, and `--` option parsing.
- [ ] Missing/invalid operand diagnostics and exact exit statuses.
- [ ] Multiple-file output headers and partial-failure ordering.
- [ ] stdin placement through `-` operands.
- [ ] Symlink, recursion, overwrite, no-clobber, metadata, and mount behavior.
- [ ] Binary input, invalid UTF-8, NUL bytes, and final-newline behavior.
- [ ] Locale-independent behavior where upstream deliberately fixes locale semantics.
- [ ] Command-specific input, output, work, allocation, and cancellation limits.
- [ ] Exact help output only for commands where upstream defines help behavior.
- [ ] Virtual executable files and quoted command names used by commands such as `xargs`.

Current rough test inventory:

- gash command-specific Go test files: approximately 52;
- pinned upstream command test files: approximately 333.

The gap is not measured by file count alone, but it shows that upstream-derived
behavioral coverage remains the largest body of work.

## Missing feature areas

### Transform API

- [ ] Bash transform parser and serializer.
- [ ] Transform plugin API and lifecycle.
- [ ] Capability restriction and revocation for transforms.
- [ ] Malformed-input, cleanup, and security tests.

### Optional language runtimes

- [ ] Select an embedded/sandboxed Python runtime; do not invoke host Python.
- [ ] Implement `python3` and `python` argv, stdin, files, imports, output,
      cancellation, memory, and cleanup behavior.
- [ ] Select an embedded/sandboxed JavaScript runtime; do not invoke host Node.js.
- [ ] Implement `js-exec` and `node` behavior with restricted globals, modules,
      filesystem, network, process APIs, time, memory, and cleanup.

### Extension boundary

- [ ] Add revocable execution scopes for custom commands and transforms.
- [ ] Prevent a completed extension from retaining live execution capabilities.
- [ ] Keep documenting that ordinary registered Go commands are trusted code and
      cannot be made safe for untrusted authors through interface restriction alone.

## Differential testing and production readiness

- [ ] Build a harness that runs identical script, filesystem, environment, stdin,
      clock, and limit fixtures against gash and pinned just-bash.
- [ ] Port upstream command fixtures with source file and test-case provenance.
- [ ] Add byte-for-byte stdout, stderr, and exit-code comparisons.
- [ ] Add parser, option-parser, path, archive, data, and SQLite fuzz corpora.
- [ ] Port upstream security and regression tests relevant to the Go architecture.
- [ ] Add stress tests for concurrent `Exec`, pipelines, mounts, SQLite locks,
      network cancellation, archives, and `xargs -P`.
- [ ] Audit all `os`, `net`, `syscall`, `unsafe`, CGO, subprocess, host environment,
      and host filesystem use.
- [ ] Add static enforcement against `os/exec` and unintended host capabilities.
- [ ] Add dependency vulnerability checks and reproducible release builds.
- [ ] Add performance benchmarks and budgets for parsing, execution, filesystem,
      search/text commands, structured data, archives, and SQLite.
- [ ] Test supported Linux, macOS, and Windows behavior.
- [ ] Finalize public API compatibility and versioning before a stable tag.

## Definition of parity-complete

A command or feature can be described as parity-complete only when:

1. its pinned upstream implementation and tests have been reviewed;
2. applicable upstream cases have Go tests with provenance;
3. stdout, stderr, status, options, operands, encoding, and errors match;
4. filesystem, mount, symlink, read-only, and mutation behavior match;
5. cancellation and all applicable limits are enforced;
6. no host capability is acquired without an explicit public capability;
7. differential tests pass, or intentional differences are documented;
8. formatting, unit tests, race tests, vet, and build checks pass.

Until those conditions hold, use `core`, `useful`, or `partial` rather than
claiming complete just-bash parity.

## Working rule

For each parity change:

1. read the pinned upstream implementation and all related tests;
2. select and document the exact behavior being ported;
3. add upstream-derived Go tests before or with the implementation;
4. keep execution inside capability-scoped Go handlers;
5. update this file and `docs/status/feature-support.json` together;
6. run `gofumpt`, tests, race tests, vet, build, and `git diff --check`;
7. commit and push the logical change.
