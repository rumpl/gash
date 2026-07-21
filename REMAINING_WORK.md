# Remaining work for `just-bash` parity

This document is the authoritative checklist for completing the Go rewrite of
[`vercel-labs/just-bash`](https://github.com/vercel-labs/just-bash).

The current repository is a **partial implementation**. A command being registered, producing useful output, or having a `--help` page does not mean it is parity-complete.

Upstream reference used for the current audit:

- repository: `vercel-labs/just-bash`
- commit: `2b316eb26b3f3e832e2cf3994d4fef160d5eb8e4`
- command registry: `packages/just-bash/src/commands/registry.ts`

## Definition of done

A feature is complete only when all applicable items below are satisfied:

- [ ] Its upstream implementation and every related upstream test have been reviewed.
- [ ] Behavior is implemented rather than delegated to a host executable.
- [ ] Success output, error output, and exit status match upstream.
- [ ] Short, long, combined, repeated, and terminating (`--`) options match upstream.
- [ ] Missing operands, invalid options, malformed input, and partial failures match upstream.
- [ ] stdin, multiple files, `-` operands, pipelines, and redirections work where applicable.
- [ ] Text, UTF-8, arbitrary bytes, NUL bytes, and final-newline behavior match where applicable.
- [ ] Virtual paths, symlinks, permissions, mounts, read-only filesystems, and mutation capabilities are respected.
- [ ] Input, output, work, recursion, allocation, and timeout limits are charged at the same boundary as upstream.
- [ ] Cancellation and fatal execution errors are propagated rather than converted into ordinary command errors.
- [ ] Security tests cover attempts to reach host processes, files, environment, network, and runtime globals.
- [ ] Ported Go tests identify the corresponding upstream test file and case.
- [ ] Differential tests compare representative scripts against upstream `just-bash`.
- [ ] `gofumpt`, unit tests, race tests, vet, and `git diff --check` pass.

## 1. Parity infrastructure

- [ ] Build an executable differential harness that runs the same script, files, environment, stdin, limits, and clock configuration against Go and upstream TypeScript.
- [ ] Normalize only intentionally nondeterministic data such as configured time; never normalize genuine output differences.
- [ ] Import upstream command test vectors and fixture files into versioned Go test data.
- [ ] Record the upstream commit beside every generated fixture.
- [ ] Add a machine-readable parity manifest with `missing`, `baseline`, `partial`, and `complete` states.
- [ ] Make CI reject a `complete` status unless the associated upstream-derived suite exists.
- [ ] Add command-wide tests for unknown options, `--`, help, stdin, missing files, cancellation, output limits, and read-only filesystems.
- [ ] Add byte-for-byte stdout/stderr comparisons, including invalid UTF-8 and NUL bytes.
- [ ] Add fuzzing for the parser boundary, command option parsers, archive readers, structured-data parsers, and filesystem paths.
- [ ] Port upstream security and regression corpora.

## 2. Shell parser and interpreter

Gash currently delegates Bash syntax and execution semantics to `mvdan.cc/sh/v3`. The following still require comparison against upstream's custom interpreter:

- [ ] Audit all lexer and parser differences.
- [ ] Audit quoting, escaping, comments, reserved words, and newline handling.
- [ ] Audit scalar, positional, special, indexed-array, and associative-array parameters.
- [ ] Audit default/alternate/error/assignment parameter operators.
- [ ] Audit prefix/suffix removal, replacement, substring, case conversion, and indirect expansion.
- [ ] Audit command substitution, arithmetic expansion, brace expansion, globbing, and pathname matching.
- [ ] Audit pipelines, `pipefail`, negation, lists, subshells, grouping, and background execution.
- [ ] Audit `if`, `case`, `for`, C-style `for`, `while`, `until`, `break`, `continue`, and `return`.
- [ ] Audit function definitions, local variables, recursion, positional parameters, and call frames.
- [ ] Audit `[[ ... ]]`, `[ ... ]`, arithmetic commands, regex matching, and test operators.
- [ ] Audit heredocs, here-strings, descriptor duplication, descriptor closing, append, clobber, and combined redirections.
- [ ] Audit process substitution support and security implications.
- [ ] Audit aliases, history, and expansion order.
- [ ] Audit traps, signals, cancellation, and exit propagation supported by upstream.
- [ ] Match `bash`, `sh`, `bash -c`, script-file, and nested-execution behavior.
- [ ] Match shell options, `set`, `shopt`, `IFS`, `BASH_*`, `SHELLOPTS`, and startup defaults.
- [ ] Replace any exposed host PID values with virtual `PID`, `PPID`, and `BASHPID` values.
- [ ] Match command-not-found, not-executable, directory-as-command, and explicit-path errors/statuses.
- [ ] Verify that no interpreter fallback opens host files or launches host processes.

## 3. Execution state, output, and limits

- [ ] Match upstream environment defaults and virtual user/process metadata.
- [ ] Match state persistence and isolation between top-level and nested executions.
- [ ] Implement upstream execution-scope cleanup and capability revocation.
- [ ] Match abort propagation through commands, pipelines, substitutions, workers, and custom commands.
- [ ] Implement text-versus-byte output-kind tracking through pipes and redirections.
- [ ] Match UTF-8/Latin-1 conversion rules used by upstream byte-clean commands.
- [ ] Match aggregate input accounting for stdin, files, nested commands, and database/archive images.
- [ ] Match maximum source length, string length, live bytes, arrays, arguments, and environment sizes.
- [ ] Match command count, loop work, recursion, nested execution, and expansion-work accounting.
- [ ] Match output limits across stdout and stderr, including concurrent pipeline writers.
- [ ] Match normal and hardened defaults field-for-field.
- [ ] Add deterministic/configurable clock support for `date`, timestamps, and timeout tests.
- [ ] Add leak tests for goroutines, open files, workers, locks, and revoked execution scopes.

## 4. Filesystems

### Public capability model

- [ ] Audit every command against the least capability it requires.
- [ ] Standardize errors returned when a capability is unavailable.
- [ ] Document atomicity guarantees for writes, rename, links, and cross-mount operations.
- [ ] Decide and test behavior for implementations that provide only subsets of writable capabilities.

### In-memory filesystem

- [ ] Differentially test path resolution, symlink traversal, loops, dangling links, and trailing slashes.
- [ ] Verify stable inode/device identity for hard links and `stat` output.
- [ ] Complete Unix mode, ownership, timestamp, and directory-link-count semantics required upstream.
- [ ] Verify quota accounting for overwrite, truncate, append, links, rename, and removal.
- [ ] Match concurrent access, lock ordering, and atomic mutation behavior.
- [ ] Match sparse/binary file behavior required by archive and SQLite support.

### Missing implementations and features

- [ ] Implement the copy-on-write overlay filesystem.
- [ ] Implement a securely rooted host read/write filesystem.
- [ ] Prevent symlink and rename escapes from a rooted host filesystem.
- [ ] Implement lazy initial files.
- [ ] Complete mount semantics for nested mounts, virtual parents, shadowing, links, and cross-device errors.
- [ ] Add mount-aware database write-back and locking.
- [ ] Add conformance suites shared by all writable filesystem implementations.

## 5. Existing commands that still require full upstream behavior

Every command in this section has some Go implementation or interpreter support, but **none should be considered complete until its upstream suites are ported and passing**.

### Basic I/O and shell builtins

- [ ] `echo` — flags, `xpg_echo`, escapes, `\c`, octal/hex/Unicode, and explicit-path invocation.
- [ ] `printf` — complete format parser, reuse rules, numeric conversion, `%b`, `%q`, `-v`, errors, and limits.
- [ ] `cat` — all transforms (`-AbenstTuv`), numbering, squeezing, byte-clean output, errors, and limits.
- [ ] `pwd` — logical/physical paths, symlinks, option parsing, and failure fallback.
- [ ] `cd` — logical/physical traversal, `CDPATH`, `OLDPWD`, environment updates, and errors.
- [ ] `bash` / `sh` — options, `-c`, script files, positional arguments, nesting, limits, and status propagation.
- [ ] `true` / `false` — exact argument/help behavior.

### File and directory commands

- [ ] `ls`
- [ ] `mkdir`
- [ ] `rmdir`
- [ ] `touch`
- [ ] `rm`
- [ ] `cp`
- [ ] `mv`
- [ ] `ln`
- [ ] `chmod`
- [ ] `readlink`
- [ ] `stat`
- [ ] `file`
- [ ] `tree`
- [ ] `du`
- [ ] `split`

For each command above, port all options, recursion rules, symlink behavior, metadata preservation, overwrite/no-clobber behavior, verbose output, multiple-operand handling, mount behavior, errors, limits, and upstream fixtures.

### Text commands

- [ ] `sed`
- [ ] `head`
- [ ] `tail`
- [ ] `wc`
- [ ] `grep`
- [ ] `egrep`
- [ ] `sort`
- [ ] `uniq`
- [ ] `tee`
- [ ] `cut`
- [ ] `paste`
- [ ] `comm`
- [ ] `join`
- [ ] `tr`
- [ ] `rev`
- [ ] `tac`
- [ ] `nl`
- [ ] `fold`
- [ ] `expand`
- [ ] `unexpand`
- [ ] `strings`
- [ ] `column`
- [ ] `od`

For each command above, port complete option parsing, locale assumptions, character/byte indexing, delimiters, regex behavior, ordering, multiple files, stdin placement, final-newline behavior, binary behavior, error ordering, and resource accounting.

### Environment, path, and miscellaneous utilities

- [ ] `env`
- [ ] `printenv`
- [ ] `basename`
- [ ] `dirname`
- [ ] `sleep`
- [ ] `seq`
- [ ] `base64`
- [ ] `md5sum`
- [ ] `sha1sum`
- [ ] `sha256sum`
- [ ] `hostname`
- [ ] `whoami`
- [ ] `clear`

For each command above, port upstream options, exact formatting, malformed-input behavior, multiple operands, configured environment/identity, cancellation, and relevant limits.

## 6. Missing standard commands

These commands exist in the audited upstream registry but are not currently implemented in Go.

### Search and text processing

- [ ] `fgrep`
- [ ] `rg`
- [ ] `awk`
- [ ] `find`
- [ ] `xargs`
- [ ] `diff`
- [ ] `expr`

### Shell state and command discovery

- [ ] `alias`
- [ ] `unalias`
- [ ] `history`
- [ ] `help`
- [ ] `which`
- [ ] `timeout`
- [ ] `time`

### Date and structured data

- [ ] `date`
- [ ] `jq`
- [ ] `yq`
- [ ] `xan`
- [ ] `html-to-markdown`

### Compression and archives

- [ ] `gzip`
- [ ] `gunzip`
- [ ] `zcat`
- [ ] `tar`

Archive work must include path traversal prevention, symlink/hard-link safety, decompression-bomb limits, entry-count limits, total expanded-byte limits, metadata handling, cancellation, malformed archive errors, and virtual-filesystem-only access.

## 7. SQLite

- [ ] Select an embedded Go SQLite implementation that does not invoke a host executable.
- [ ] Decide whether CGO is acceptable; document and test the build strategy on supported platforms.
- [ ] Implement `:memory:` databases.
- [ ] Load database images from the virtual filesystem.
- [ ] Persist mutated database images back through filesystem capabilities.
- [ ] Implement atomic write-back and preserve the original database on failure.
- [ ] Implement per-database locking and match concurrent update behavior.
- [ ] Make locking mount- and symlink-aware.
- [ ] Support multiple SQL statements and SQLite-driven statement boundary parsing.
- [ ] Match non-bail recovery and `-bail` behavior.
- [ ] Implement `-readonly`, `-cmd`, `-echo`, `-header`, `-noheader`, `-separator`, `-newline`, `-nullvalue`, and `-version`.
- [ ] Implement list, CSV, JSON, line, column, table, Markdown, tabs, box, quote, HTML, and ASCII output modes.
- [ ] Match integer, float, text, BLOB, and NULL formatting.
- [ ] Match SQL parse/runtime error text and exit status.
- [ ] Disable extension loading and all host-filesystem escape mechanisms.
- [ ] Enforce database image, query output, statement work, memory, and execution-time limits.
- [ ] Propagate cancellation and cleanly interrupt active queries.
- [ ] Port all upstream SQLite fixtures and test files, including worker-protocol, lock-abort, UTF-8 stdin, resource-limit, and write-back regressions.

## 8. Networking

- [ ] Add opt-in `curl`; network access must remain unavailable by default.
- [ ] Implement the upstream URL allow policy for scheme, origin, port, path, and method.
- [ ] Validate every redirect rather than only the initial URL.
- [ ] Restrict request headers and transformed headers.
- [ ] Enforce request/response byte limits, redirect limits, and timeouts.
- [ ] Handle DNS rebinding and private/link-local/loopback address policy as upstream requires.
- [ ] Stream through bounded buffers and propagate cancellation.
- [ ] Keep credentials and host environment inaccessible.
- [ ] Port upstream network policy, redirect, transform, and security tests.

## 9. Optional runtimes

These must be explicit opt-ins and are a larger security boundary than ordinary commands.

### Python

- [ ] Implement `python3` and `python` with an embedded/sandboxed runtime rather than host `python`.
- [ ] Match `-c`, `-m`, script-file, argv, stdin/stdout/stderr, and exit behavior.
- [ ] Bridge only the intended virtual filesystem and environment.
- [ ] Enforce timeout, memory, output, import, and cleanup limits.
- [ ] Port upstream CPython worker and security tests.

### JavaScript

- [ ] Implement `js-exec` and the upstream `node` stub behavior with an embedded runtime rather than host Node.js.
- [ ] Restrict globals, modules, dynamic imports, network, filesystem, process APIs, and WebAssembly as required.
- [ ] Enforce timeout, memory, output, and cleanup limits.
- [ ] Port upstream QuickJS worker and security tests.

## 10. Transform API

- [ ] Port transform parsing and serialization.
- [ ] Port the transform plugin API and lifecycle.
- [ ] Port Bash serializer behavior.
- [ ] Implement capability restriction and revocation for transforms.
- [ ] Port transform fixtures, malformed-input tests, and security tests.

## 11. Help, registration, and packaging

- [x] Add shared structured help formatting.
- [x] Import upstream help definitions for currently registered commands that define help.
- [ ] Move help definitions beside each command rather than relying on a generated central catalog.
- [ ] Match commands that intentionally treat `--help` as data or an error; do not globally invent help behavior.
- [ ] Implement upstream `help` command and command listing.
- [ ] Match aliases (`fgrep`, `egrep`, `sh`, `python`, `node`, `gunzip`, and `zcat`) exactly.
- [ ] Implement optional registration for network and language runtimes.
- [ ] Match command filtering, lazy initialization where relevant, and registry introspection.
- [ ] Give every command one source file and an adjacent command-specific test file.
- [ ] Move remaining basic I/O, environment, path, checksum, and miscellaneous commands into dedicated family packages.
- [ ] Remove dead or duplicate command implementations after migration.

## 12. Public API and CLI

- [ ] Compare every public `just-bash` option with the Go API and document intentional Go-specific differences.
- [ ] Finalize stable constructors, option types, command registration, filesystem injection, and result types.
- [ ] Add public support for network policy, transforms, optional runtimes, deterministic time, and lazy files.
- [ ] Define backward-compatibility policy before a stable release; do not retain accidental pre-release compatibility aliases.
- [x] Add explicit read-only `--root DIR` CLI access for trusted local use via `os.DirFS`.
- [ ] Replace or supplement `--root` with a symlink-safe rooted host filesystem for untrusted use.
- [ ] Match CLI `-c`, script-file, stdin, environment, cwd, exit status, and signal behavior.
- [ ] Add CLI help/version and complete invalid-invocation tests.
- [ ] Add black-box tests against a built `gash` binary.
- [ ] Test Linux, macOS, and Windows build/runtime behavior where supported.
- [ ] Document unsupported shell/platform behavior explicitly.

## 13. Security and production readiness

- [ ] Complete a threat model for filesystem, custom commands, network, archives, SQLite, transforms, and optional runtimes.
- [ ] Audit all uses of `os`, `net`, `syscall`, `unsafe`, CGO, subprocess APIs, and host environment access.
- [ ] Add static checks that reject `os/exec` and unintended host filesystem/network access in built-ins.
- [ ] Make custom-command execution scopes revocable after command completion.
- [ ] Document that registered Go commands are trusted native code, not sandboxed code.
- [ ] Add denial-of-service tests for CPU, memory, output, recursion, goroutines, locks, and pathological inputs.
- [ ] Add race and deadlock stress tests for pipelines, mounts, database locks, cancellation, and concurrent `Exec` calls.
- [ ] Add dependency scanning, vulnerability reporting, and reproducible release builds.
- [ ] Add benchmarks and enforce budgets for parser, interpreter, filesystem, grep/sort, archives, and SQLite.
- [ ] Establish supported Go versions and a release/versioning policy.
- [ ] Complete API documentation and runnable examples.
- [ ] Review license notices for copied/adapted fixtures and behavior tables.

## 14. Final release gate

- [ ] Every upstream registry command is implemented or explicitly documented as an intentional exclusion.
- [ ] Every command marked complete passes its upstream-derived and differential suites.
- [ ] Shell semantics pass the agreed upstream compatibility corpus.
- [ ] Filesystem implementations pass standard and project-specific conformance suites.
- [ ] Normal and hardened profiles pass resource-exhaustion tests.
- [ ] No built-in can launch a host process or access host files/network without an explicit capability.
- [ ] Full tests, race tests, fuzz smoke tests, vet, formatting, vulnerability checks, and platform builds pass in CI.
- [ ] Documentation accurately distinguishes complete, partial, optional, and unsupported behavior.
- [ ] A security review has been completed.
- [ ] The module is tagged only after all release-gate items are satisfied.
