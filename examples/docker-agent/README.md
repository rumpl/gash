# Docker Agent with a gash shell tool

This example embeds gash as the `shell` tool of a
[Docker Agent](https://github.com/docker/cagent) Go application. The model can
inspect files and run gash commands, but it cannot launch host executables.
Docker Agent framework logs are discarded; the terminal shows streamed assistant
text, complete shell tool calls, and their structured results. Interactive output
uses colors automatically; use `-color always` when piping a demonstration or
`-color never` to disable ANSI styling.

The example is a nested Go module because Docker Agent has a large, independently
versioned dependency graph and a newer Go toolchain requirement than the core
gash module.

## Run

Set an OpenAI API key, then ask the agent a question:

```sh
cd examples/docker-agent
export OPENAI_API_KEY=...
go run . "What kind of project is in this directory? Use the shell to inspect it."
```

The current directory is exposed as `/` inside gash. By default the wrapper only
implements `io/fs.FS`, so mutation capabilities are hidden and the agent sees a
**read-only** host filesystem.

Choose another host directory:

```sh
go run . -root ../.. "List the Go packages and summarize their purpose."
```

## Explicitly allow writes

`-writable` passes the full `fs.Rooted` capability set to gash. Commands can then
modify files under `-root`:

```sh
go run . -root /tmp/agent-work -writable \
  "Create notes.txt containing a short project summary, then show it to me."
```

Use `-writable` only when the agent should be allowed to change the selected
directory. Host commands are still unavailable.

## Explicitly allow network access

Network access and `curl` are absent by default. Enable selected HTTP(S) origins:

```sh
go run . \
  -network-allow https://api.github.com,https://example.com/docs \
  "Fetch the allowed documentation URL and summarize it."
```

The policy checks scheme, host, port, path, redirects, response size, and resolved
addresses. Private, loopback, and link-local addresses remain denied by default.

## Other flags

```text
-color string
      color output: auto, always, or never (default "auto")
-model string
      OpenAI model used by docker-agent (default "gpt-4o")
-prompt string
      question or task for the agent
-root string
      host directory exposed as / inside gash (default ".")
-writable
      allow shell commands to modify files under -root
-network-allow string
      comma-separated HTTP(S) origins allowed for curl
```

The shell uses gash's hardened limit profile. Tool results are JSON objects with
`stdout`, `stderr`, and `exit_code`, and the agent is instructed to check failures
rather than assuming command success. Assistant text is printed chunk-by-chunk as
Docker Agent emits it, while calls and results are separated into labeled blocks. The example pre-approves tool calls so it
can run non-interactively; its safety boundary is the read-only filesystem,
disabled-by-default network, hardened limits, and lack of host-process fallback.
