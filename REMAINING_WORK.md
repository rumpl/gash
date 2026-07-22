# Remaining work for practical just-bash-inspired gash

Gash is a pragmatic Go rewrite inspired by
[`vercel-labs/just-bash`](https://github.com/vercel-labs/just-bash). The goal is to
provide useful, capability-scoped shell workflows in Go without launching host
processes or exposing the host filesystem by default. We use upstream just-bash
as implementation guidance for selected tasks, but this document is not a
promise to clone every upstream command, test, runtime, or edge case before gash
is useful or releasable.

Pinned upstream reference for current planning:

- repository/commit: <https://github.com/vercel-labs/just-bash/tree/2b316eb26b3f3e832e2cf3994d4fef160d5eb8e4>
- command registry: <https://github.com/vercel-labs/just-bash/blob/2b316eb26b3f3e832e2cf3994d4fef160d5eb8e4/packages/just-bash/src/commands/registry.ts>
- gash feature support manifest: [`docs/status/feature-support.json`](docs/status/feature-support.json)
- implementation root: <https://github.com/vercel-labs/just-bash/tree/2b316eb26b3f3e832e2cf3994d4fef160d5eb8e4/packages/just-bash/src>

When a task selects an upstream feature, implement what just-bash implements for
that feature unless the task explicitly documents an intentional Go-specific
simplification or deferral. Prefer small, testable slices over broad compatibility
audits.

The feature support manifest is the product source of truth for command-level
status. It uses these statuses: `core` for dependable baseline features,
`useful` for practical in-process subsets, `partial` for present but explicitly
limited behavior, `optional` for future opt-in capabilities/runtimes, `deferred`
for tracked upstream items not implemented yet, and `unsupported` for features
intentionally outside the current product surface.

## Near-term priority: search and text commands

The next practical milestone is to make common agent workflows work well with
search, filtering, and text transformation. Treat these as first-class tasks,
not as one large bucket:

| Command | Priority goal | Upstream guidance |
| --- | --- | --- |
| `fgrep` | Ensure the fixed-string grep alias/behavior is registered, documented, and tested against the current `grep` implementation. | [`commands/registry.ts`](https://github.com/vercel-labs/just-bash/blob/2b316eb26b3f3e832e2cf3994d4fef160d5eb8e4/packages/just-bash/src/commands/registry.ts), upstream grep command source/tests under [`commands/`](https://github.com/vercel-labs/just-bash/tree/2b316eb26b3f3e832e2cf3994d4fef160d5eb8e4/packages/just-bash/src/commands) |
| `sed` | Add the common stream-editing subset needed by scripts, with clear unsupported-expression errors. | upstream `sed` command source/tests under [`commands/`](https://github.com/vercel-labs/just-bash/tree/2b316eb26b3f3e832e2cf3994d4fef160d5eb8e4/packages/just-bash/src/commands) |
| `find` | Expand the existing baseline around predicates/actions used by agent workflows. | upstream `find` command source/tests under [`commands/`](https://github.com/vercel-labs/just-bash/tree/2b316eb26b3f3e832e2cf3994d4fef160d5eb8e4/packages/just-bash/src/commands) |
| `xargs` | Implemented with stdin-to-argv batching, replacement, delimiters, bounded parallelism, and virtual command execution; additional GNU flags and quoting semantics remain deferred. | upstream `xargs` command source/tests under [`commands/`](https://github.com/vercel-labs/just-bash/tree/2b316eb26b3f3e832e2cf3994d4fef160d5eb8e4/packages/just-bash/src/commands) |
| `diff` | Implemented just-bash-inspired file/stdin comparison, status handling, brief/identical/ignore-case flags, and unified diff output. Large-file hunk grouping may differ from upstream jsdiff formatting. | upstream `diff` command source/tests under [`commands/diff`](https://github.com/vercel-labs/just-bash/tree/2b316eb26b3f3e832e2cf3994d4fef160d5eb8e4/packages/just-bash/src/commands/diff) |
| `expr` | Implement expression evaluation commonly used by shell scripts. | upstream `expr` command source/tests under [`commands/`](https://github.com/vercel-labs/just-bash/tree/2b316eb26b3f3e832e2cf3994d4fef160d5eb8e4/packages/just-bash/src/commands) |
| `rg` | Provide a practical ripgrep-like search command over the virtual filesystem. | upstream `rg` command source/tests under [`commands/`](https://github.com/vercel-labs/just-bash/tree/2b316eb26b3f3e832e2cf3994d4fef160d5eb8e4/packages/just-bash/src/commands) |
| `awk` | Add a useful AWK subset or embedded implementation decision, with explicit limits. | upstream `awk` command source/tests under [`commands/`](https://github.com/vercel-labs/just-bash/tree/2b316eb26b3f3e832e2cf3994d4fef160d5eb8e4/packages/just-bash/src/commands) |

For each priority command:

- read the pinned upstream implementation and command-specific tests before
  choosing the Go behavior;
- add Go tests for the selected behavior, including stdin, files, pipelines,
  virtual paths, read-only filesystems, cancellation, and output limits where
  applicable;
- document unsupported flags or edge cases in the command help or tests rather
  than implying they work;
- keep all execution inside gash command handlers; do not delegate to host
  executables.

## Current useful baseline

Gash already has a useful Bash AST interpreter, virtual filesystem model, limits,
and a set of built-in commands. Existing commands can be improved incrementally
without blocking the priority search/text work.

Implemented or partially implemented command families include:

- basic I/O: `echo`, `printf`, `cat`
- navigation: `cd`, `pwd`
- files: `ls`, `mkdir`, `rmdir`, `touch`, `rm`, `cp`, `mv`, `ln`, `readlink`,
  `chmod`, `stat`, `file`, `tree`, `du`, `split`
- text baseline: `head`, `tail`, `wc`, `grep`/`egrep`/`fgrep`, `sort`, `uniq`,
  `tee`, `cut`, `paste`, `comm`, `diff`, `join`, `tr`, `rev`, `tac`, `nl`, `fold`,
  `expand`, `unexpand`, `strings`, `column`, `od`, `xargs`
- environment/path: `env`, `printenv`, `basename`, `dirname`
- utility subset: `true`, `false`, `sleep`, `seq`, `base64`, checksums,
  `hostname`, `whoami`, `clear`

Use upstream tests for these commands as guidance when a task touches them, but
do not treat every unported flag as a release blocker. If behavior intentionally
differs from upstream, document the difference in the relevant command tests,
README, help text, or porting notes.

## Feature-area backlog

### Shell parser and interpreter

Gash uses `mvdan.cc/sh/v3` for Bash parsing and execution structure. Practical
work should focus on behavior that affects real scripts and gash's security
model.

Upstream guidance:

- parser and AST: [`parser/`](https://github.com/vercel-labs/just-bash/tree/2b316eb26b3f3e832e2cf3994d4fef160d5eb8e4/packages/just-bash/src/parser), [`ast/`](https://github.com/vercel-labs/just-bash/tree/2b316eb26b3f3e832e2cf3994d4fef160d5eb8e4/packages/just-bash/src/ast)
- interpreter: [`interpreter/`](https://github.com/vercel-labs/just-bash/tree/2b316eb26b3f3e832e2cf3994d4fef160d5eb8e4/packages/just-bash/src/interpreter)
- core execution types: [`Bash.ts`](https://github.com/vercel-labs/just-bash/blob/2b316eb26b3f3e832e2cf3994d4fef160d5eb8e4/packages/just-bash/src/Bash.ts), [`execution-scope.ts`](https://github.com/vercel-labs/just-bash/blob/2b316eb26b3f3e832e2cf3994d4fef160d5eb8e4/packages/just-bash/src/execution-scope.ts), [`execution-output.ts`](https://github.com/vercel-labs/just-bash/blob/2b316eb26b3f3e832e2cf3994d4fef160d5eb8e4/packages/just-bash/src/execution-output.ts), [`limits.ts`](https://github.com/vercel-labs/just-bash/blob/2b316eb26b3f3e832e2cf3994d4fef160d5eb8e4/packages/just-bash/src/limits.ts)

Backlog:

- audit shell behaviors that commonly affect scripts: quoting, parameter
  expansion, arrays, arithmetic, command substitution, globbing, redirections,
  heredocs, pipelines, conditionals, loops, functions, and nested `bash`/`sh`;
- replace exposed host process metadata with virtual values where needed;
- improve cancellation and fatal-error propagation through pipelines and nested
  execution;
- document shell features intentionally delegated to `mvdan.cc/sh/v3` semantics.

### Filesystems

The virtual filesystem boundary is a core gash feature. Prioritize correctness,
capability checks, and safe behavior over matching every upstream internal data
structure.

Upstream guidance:

- filesystem contracts and implementations: [`fs/`](https://github.com/vercel-labs/just-bash/tree/2b316eb26b3f3e832e2cf3994d4fef160d5eb8e4/packages/just-bash/src/fs)
- command registry and command tests that exercise filesystem behavior: [`commands/registry.ts`](https://github.com/vercel-labs/just-bash/blob/2b316eb26b3f3e832e2cf3994d4fef160d5eb8e4/packages/just-bash/src/commands/registry.ts)

Backlog:

- continue conformance tests for read-only, writable, and mountable `io/fs`
  implementations;
- improve symlink, hard-link, metadata, rename, quota, and cross-mount behavior
  when command work requires it;
- implement a copy-on-write overlay filesystem if a concrete use case needs it;
- implement a symlink-safe rooted host filesystem before documenting host writes
  for untrusted use;
- document atomicity and capability requirements for mutations.

### Existing command improvements

When improving an existing command, consult its upstream command implementation
and tests under the pinned [`commands/`](https://github.com/vercel-labs/just-bash/tree/2b316eb26b3f3e832e2cf3994d4fef160d5eb8e4/packages/just-bash/src/commands)
directory. Also check shared helpers near the command registry.

Backlog:

- add focused upstream-derived tests for flags and edge cases that users need;
- improve error text, status codes, stdin/file ordering, binary data handling,
  final-newline behavior, and read-only filesystem failures as tasks require;
- keep command implementations capability-scoped and free of `os/exec`;
- move command-specific help and tests next to the command implementation when
  practical.

### Networking

Network access should remain opt-in. Implement only with an explicit API and a
policy that keeps credentials and host environment unavailable by default.

Upstream guidance:

- network policy and curl implementation: [`network/`](https://github.com/vercel-labs/just-bash/tree/2b316eb26b3f3e832e2cf3994d4fef160d5eb8e4/packages/just-bash/src/network), command sources/tests under [`commands/`](https://github.com/vercel-labs/just-bash/tree/2b316eb26b3f3e832e2cf3994d4fef160d5eb8e4/packages/just-bash/src/commands)

Backlog:

- design the Go network policy API before adding `curl`;
- validate redirects, headers, methods, origins, private-address policy, byte
  limits, and cancellation;
- document that network commands are unavailable unless explicitly enabled.

### Structured data, archives, and SQLite

These features are useful but are not ahead of the search/text priority unless a
specific product need appears.

Upstream guidance:

- command implementations/tests under [`commands/`](https://github.com/vercel-labs/just-bash/tree/2b316eb26b3f3e832e2cf3994d4fef160d5eb8e4/packages/just-bash/src/commands)
- SQLite-related worker/protocol code where present under the pinned source tree

Backlog:

- `jq`, `yq`, `xan`, and related structured-data commands;
- `gzip`, `gunzip`, `zcat`, and `tar` with traversal and decompression-bomb
  protections;
- `sqlite3` via an embedded implementation, virtual-filesystem-only access,
  write-back semantics, locking, cancellation, and output modes.

### Optional language runtimes

Python and JavaScript runtimes are larger security boundaries and should remain
explicit opt-ins.

Upstream guidance:

- worker/runtime boundaries under the pinned [`packages/just-bash/src`](https://github.com/vercel-labs/just-bash/tree/2b316eb26b3f3e832e2cf3994d4fef160d5eb8e4/packages/just-bash/src) tree
- command sources/tests under [`commands/`](https://github.com/vercel-labs/just-bash/tree/2b316eb26b3f3e832e2cf3994d4fef160d5eb8e4/packages/just-bash/src/commands)

Backlog:

- decide on embedded/sandboxed runtimes rather than host `python` or Node.js;
- bridge only explicit virtual filesystem, environment, stdio, and time/limit
  capabilities;
- enforce cleanup, memory, output, import/module, and timeout limits.

### Transform API and Bash serialization

Transform support is optional until a concrete integration needs it.

Upstream guidance:

- transform parsing, serialization, and plugins: [`transform/`](https://github.com/vercel-labs/just-bash/tree/2b316eb26b3f3e832e2cf3994d4fef160d5eb8e4/packages/just-bash/src/transform)

Backlog:

- port transform parsing/serialization when needed;
- define plugin capability restrictions and revocation;
- add malformed-input and security tests for selected transform behavior.

### Public API, CLI, and packaging

Public API and CLI work should make current capabilities clear rather than imply
unimplemented upstream behavior.

Upstream guidance:

- public entry points and types: [`Bash.ts`](https://github.com/vercel-labs/just-bash/blob/2b316eb26b3f3e832e2cf3994d4fef160d5eb8e4/packages/just-bash/src/Bash.ts), [`types.ts`](https://github.com/vercel-labs/just-bash/blob/2b316eb26b3f3e832e2cf3994d4fef160d5eb8e4/packages/just-bash/src/types.ts)
- registry behavior: [`commands/registry.ts`](https://github.com/vercel-labs/just-bash/blob/2b316eb26b3f3e832e2cf3994d4fef160d5eb8e4/packages/just-bash/src/commands/registry.ts)

Backlog:

- document supported commands, partial behavior, and intentional deferrals;
- finalize constructors, options, command registration, filesystem injection,
  result types, and compatibility policy before a stable tag;
- add CLI help/version and black-box CLI tests;
- keep README, PORTING, command help, and tests aligned.

## Working rule for new tasks

1. Pick a narrow behavior slice, preferably from the priority search/text list.
2. Link to the pinned upstream source and test files that informed the choice.
3. Implement the selected behavior in Go without host command delegation.
4. Add focused tests for expected behavior and documented deferrals.
5. Run formatting and the smallest meaningful validation command set.

This plan should change as gash users reveal which shell workflows matter most.
Prefer accurate documentation of supported behavior over broad compatibility
claims.
