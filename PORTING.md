# just-bash-inspired Go rewrite plan

This file summarizes the Go port against selected behavior from
`vercel-labs/just-bash/packages/just-bash/src`. A checked item means a tested
baseline is implemented; it does **not** claim complete upstream flag or
edge-case coverage. For selected tasks, implement what pinned just-bash
implements unless an intentional deferral is documented. The practical backlog
is [REMAINING_WORK.md](REMAINING_WORK.md).

## Upstream architecture reviewed

The port is based on the pinned upstream implementation and tests when a task
selects that feature. Useful references include:

- `Bash.ts`, `types.ts`, `limits.ts`, `execution-scope.ts`, and `execution-output.ts`
- `ast/types.ts`, the lexer/parser under `parser/`, and the interpreter under `interpreter/`
- command registration in `commands/registry.ts` and the command-specific test suites
- the filesystem contract and the in-memory, overlay, read-write, and mountable implementations under `fs/`
- network policy and curl under `network/` and `commands/curl/`
- transform parsing/serialization/plugins under `transform/`
- the Python, QuickJS, and SQLite worker boundaries
- security, source/input/output limits, abort propagation, encoding, and custom-command revocation

The upstream currently contains roughly 1,545 TypeScript source/test files, 133 interpreter files, and 173 non-test command files. Porting is organized by behavioral boundary rather than by copying TypeScript file-for-file.

## Shell and execution

- [x] Bash AST parser rather than token splitting
- [x] quoting and parameter expansion
- [x] pipelines, lists, logical operators, and redirections
- [x] functions, conditionals, loops, arithmetic, command substitution, and heredocs
- [x] isolated shell state per `Exec`; shared filesystem across calls
- [x] positional arguments supplied without source-string escaping
- [x] context deadline and aggregate stdout/stderr output budget
- [x] shared source, input, command, and nested-execution limits
- [x] normal and hardened top-level limit profiles
- [x] virtual UID/EUID/GID defaults
- [ ] exact upstream environment, shell option, and process metadata defaults
- [ ] string/live-byte/work/loop/archive/database limit accounting
- [ ] abort propagation and extension cleanup/revocation semantics
- [ ] byte/text output-kind tracking matching upstream's encoding pipeline
- [ ] transform plugin API and Bash serializer

The parser/interpreter is provided by `mvdan.cc/sh/v3`; gash owns all filesystem and command handlers so no host process is launched and no host filesystem handler is used. Remaining shell work is prioritized by practical script behavior and security impact rather than by a requirement to reproduce every upstream internal detail.

## Filesystems

- [x] `io/fs.FS` as the base public contract
- [x] granular writable capability interfaces
- [x] bounded in-memory implementation
- [x] standard `fstest.TestFS` conformance
- [x] support for arbitrary standard read-only implementations
- [x] files, directories, symlinks, rename, recursive removal, and quota accounting
- [x] hard-link, chmod, and timestamp capabilities
- [ ] stable inode identity and metadata behavior for selected command needs
- [ ] copy-on-write overlay implementation
- [ ] rooted read-write host implementation
- [x] mountable filesystem, virtual parent directories, and cross-mount copy/move
- [ ] lazy initial files

## Commands

Current initial implementations:

- [x] basic I/O: `echo`, `printf`, `cat`
- [x] navigation: `cd`, `pwd`
- [x] files: `ls`, `mkdir`, `rmdir`, `touch`, `rm`, recursive `cp`, `mv`, symbolic/hard `ln`, `readlink`, `chmod`, `stat`, `file`, `tree`, `du`, `split`
- [x] text baseline: `head`, `tail`, `wc`, `grep`/`egrep`/`fgrep`, `sort`, `uniq`, `tee`, `cut`, `paste`, `comm`, `diff`, `join`, `tr`, `rev`, `tac`, `nl`, `fold`, `expand`, `unexpand`, `strings`, `column`, `od`
- [x] environment/path: `env`, `printenv`, `basename`, `dirname`
- [x] utility subset: `true`, `false`, `sleep`, `seq`, `base64`, checksums, `hostname`, `whoami`, `clear`

The checks above mean a useful baseline exists. Exact GNU/BSD flag behavior is
not promised unless a task selects that behavior and adds tests from or inspired
by the pinned upstream command suite.

Near-term search/text priorities:

- [ ] `fgrep` — verify fixed-string alias behavior against `grep` and document gaps
- [ ] `sed`
- [ ] `find` — expand the existing baseline around common predicates/actions
- [x] `xargs` — in-process batching, replacement, delimiters, parallel execution, and virtual command dispatch; additional flags remain deferred
- [x] `diff` — just-bash-inspired file/stdin comparison with `-u`, `-q`, `-s`, and `-i`; Go-native unified hunk grouping may differ from upstream jsdiff output for larger files.
- [ ] `expr`
- [ ] `rg`
- [ ] `awk`

Broader optional command work:

- [ ] remaining file-command edge cases and additional GNU/BSD flags when needed
- [ ] remaining text utility flags and binary/locale edge cases
- [ ] aliases, history, shell wrappers, and timeout
- [ ] `jq`, `yq`, `xan`, `sqlite3`, gzip, tar, and file detection
- [ ] secure `curl` and HTML-to-Markdown
- [ ] opt-in CPython and QuickJS runtimes

## Security backlog

- [x] no `os/exec` path in gash command execution
- [x] no default host filesystem visibility
- [x] read-only capability failure for standard read-only filesystems
- [x] filesystem and output bounds
- [ ] virtual PID/PPID/BASHPID (the shell dependency currently exposes host process IDs)
- [ ] URL origin/path/method/redirect/header-transform firewall
- [ ] source, expansion, loop, recursion, archive, and database limits selected for supported features
- [ ] restricted custom-command boundary and cleanup scope
- [ ] fuzz corpus and upstream comparison harness

## Porting rule

A selected feature is implemented by first reading its pinned upstream source and
tests, then adding Go behavior tests derived from the chosen cases, then
implementing against `io/fs` capabilities. Intentional simplifications or
deferrals should be documented in tests, help text, README, or REMAINING_WORK so
future tasks do not mistake them for accidental gaps.
