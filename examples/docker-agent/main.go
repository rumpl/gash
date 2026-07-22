package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	iofs "io/fs"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/term"

	"github.com/docker/docker-agent/pkg/agent"
	"github.com/docker/docker-agent/pkg/config/latest"
	"github.com/docker/docker-agent/pkg/environment"
	"github.com/docker/docker-agent/pkg/model/provider/openai"
	"github.com/docker/docker-agent/pkg/runtime"
	"github.com/docker/docker-agent/pkg/session"
	"github.com/docker/docker-agent/pkg/team"
	"github.com/docker/docker-agent/pkg/tools"
	gashfs "github.com/rumpl/gash/pkg/fs"
	"github.com/rumpl/gash/pkg/gash"
	"github.com/rumpl/gash/pkg/network"
)

type options struct {
	root         string
	writable     bool
	networkAllow string
	model        string
	prompt       string
	color        string
}

type readOnlyFS struct {
	filesystem iofs.FS
}

func (r readOnlyFS) Open(name string) (iofs.File, error) {
	return r.filesystem.Open(name)
}

type shellArgs struct {
	Cmd     string `json:"cmd" jsonschema:"Bash command to execute inside gash"`
	Cwd     string `json:"cwd,omitempty" jsonschema:"Virtual working directory; defaults to /"`
	Stdin   string `json:"stdin,omitempty" jsonschema:"Optional standard input for the command"`
	Timeout int    `json:"timeout,omitempty" jsonschema:"Optional timeout in seconds"`
}

type shellOutput struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
}

func main() {
	// Docker Agent logs through slog. This example renders the event stream
	// explicitly, so framework logs would only obscure the conversation.
	slog.SetDefault(slog.New(slog.DiscardHandler))

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	configured := parseFlags()
	if err := run(ctx, configured); err != nil {
		log.Fatal(err)
	}
}

func parseFlags() options {
	root := flag.String("root", ".", "host directory exposed as / inside gash")
	writable := flag.Bool("writable", false, "allow shell commands to modify files under -root")
	networkAllow := flag.String("network-allow", "", "comma-separated HTTP(S) origins allowed for curl")
	model := flag.String("model", "gpt-5.6-sol", "OpenAI model used by docker-agent")
	prompt := flag.String("prompt", "", "question or task for the agent; remaining arguments are also accepted")
	color := flag.String("color", "auto", "color output: auto, always, or never")
	flag.Parse()

	question := strings.TrimSpace(*prompt)
	if question == "" {
		question = strings.TrimSpace(strings.Join(flag.Args(), " "))
	}
	if question == "" {
		question = "Inspect the current directory with the shell tool and summarize what this project does."
	}
	return options{
		root:         *root,
		writable:     *writable,
		networkAllow: *networkAllow,
		model:        *model,
		prompt:       question,
		color:        *color,
	}
}

func run(ctx context.Context, configured options) error {
	shell, root, err := newShell(configured)
	if err != nil {
		return err
	}

	llm, err := openai.NewClient(
		ctx,
		&latest.ModelConfig{
			Provider: "openai",
			Model:    configured.model,
		},
		environment.NewDefaultProvider(),
	)
	if err != nil {
		return err
	}

	shellTool := tools.Tool{
		Name:       "shell",
		Category:   "shell",
		Parameters: tools.MustSchemaFor[shellArgs](),
		Description: "Execute Bash commands with gash. The host directory is mounted at / " +
			"without host process execution. Files are read-only unless this program starts with -writable.",
		Handler: shellHandler(shell),
	}

	filesystemMode := "read-only"
	if configured.writable {
		filesystemMode = "writable"
	}
	instructions := fmt.Sprintf(`You are a coding agent with a capability-scoped shell tool.
Use the shell tool whenever filesystem inspection or command execution would help.
The host directory %q is visible as / and is %s.
Never claim a command succeeded without checking its exit_code and stderr.
Host executables are unavailable. Network access is unavailable unless curl was explicitly enabled.`, root, filesystemMode)

	worker := agent.New(
		"root",
		instructions,
		agent.WithDescription("A coding agent using an isolated in-process gash shell."),
		agent.WithModel(llm),
		agent.WithTools(shellTool),
	)
	runtimeTeam := team.New(team.WithAgents(worker))
	rt, err := runtime.New(ctx, runtimeTeam)
	if err != nil {
		return err
	}

	sess := session.New(
		session.WithUserMessage(configured.prompt),
		session.WithToolsApproved(true),
	)
	colors, err := colorEnabled(configured.color, os.Stdout)
	if err != nil {
		return err
	}
	printer := newStreamPrinter(os.Stdout, os.Stderr, colors)
	var streamErr error
	for event := range rt.RunStream(ctx, sess) {
		switch typed := event.(type) {
		case *runtime.AgentChoiceEvent:
			printer.writeAssistant(typed.Content)
		case *runtime.ToolCallEvent:
			printer.writeToolCall(typed.ToolCall)
		case *runtime.ToolCallResponseEvent:
			printer.writeToolResult(typed.ToolDefinition.Name, typed.Result, typed.Response)
		case *runtime.ToolCallConfirmationEvent:
			// The session is pre-approved, but approve defensively if a model or
			// runtime configuration still requests confirmation.
			rt.Resume(ctx, runtime.ResumeApproveSession())
		case *runtime.ErrorEvent:
			streamErr = fmt.Errorf("%s", typed.Error)
			printer.writeError(typed.Error)
		}
	}
	printer.finish()
	return streamErr
}

const (
	ansiReset   = "\x1b[0m"
	ansiBold    = "\x1b[1m"
	ansiDim     = "\x1b[2m"
	ansiRed     = "\x1b[31m"
	ansiGreen   = "\x1b[32m"
	ansiMagenta = "\x1b[35m"
	ansiCyan    = "\x1b[36m"
)

type streamPrinter struct {
	stdout        io.Writer
	stderr        io.Writer
	colors        bool
	assistantOpen bool
	wroteSection  bool
}

func newStreamPrinter(stdout, stderr io.Writer, colors bool) *streamPrinter {
	return &streamPrinter{stdout: stdout, stderr: stderr, colors: colors}
}

func (p *streamPrinter) writeAssistant(content string) {
	if content == "" {
		return
	}
	if !p.assistantOpen {
		p.startSection()
		fmt.Fprint(p.stdout, p.style(ansiBold+ansiCyan, "assistant>"), " ")
		p.assistantOpen = true
	}
	fmt.Fprint(p.stdout, content)
}

func (p *streamPrinter) writeToolCall(toolCall tools.ToolCall) {
	p.closeAssistant()
	p.startSection()
	fmt.Fprintf(p.stdout, "%s %s\n", p.style(ansiBold+ansiMagenta, "tool call>"), toolCall.Function.Name)
	fmt.Fprintln(p.stdout, p.style(ansiDim, indent(prettyJSON(toolCall.Function.Arguments))))
}

func (p *streamPrinter) writeToolResult(name string, result *tools.ToolCallResult, response string) {
	p.closeAssistant()
	p.startSection()
	fmt.Fprintf(p.stdout, "%s %s\n", p.style(ansiBold+ansiGreen, "tool result>"), name)
	value := response
	if result != nil {
		value = result.Output
	}
	fmt.Fprintln(p.stdout, p.style(ansiDim, indent(prettyJSON(value))))
}

func (p *streamPrinter) writeError(message string) {
	p.closeAssistant()
	fmt.Fprintf(p.stderr, "%s %s\n", p.style(ansiBold+ansiRed, "error:"), message)
}

func (p *streamPrinter) finish() {
	p.closeAssistant()
}

func (p *streamPrinter) style(code, value string) string {
	if !p.colors {
		return value
	}
	return code + value + ansiReset
}

func (p *streamPrinter) startSection() {
	if p.wroteSection {
		fmt.Fprintln(p.stdout)
	}
	p.wroteSection = true
}

func (p *streamPrinter) closeAssistant() {
	if !p.assistantOpen {
		return
	}
	fmt.Fprintln(p.stdout)
	p.assistantOpen = false
}

func colorEnabled(mode string, output io.Writer) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "always":
		return true, nil
	case "never":
		return false, nil
	case "", "auto":
		if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
			return false, nil
		}
		file, ok := output.(*os.File)
		return ok && term.IsTerminal(int(file.Fd())), nil
	default:
		return false, fmt.Errorf("invalid -color value %q: use auto, always, or never", mode)
	}
}

func prettyJSON(value string) string {
	var decoded any
	if err := json.Unmarshal([]byte(value), &decoded); err != nil {
		return value
	}
	formatted, err := json.MarshalIndent(decoded, "", "  ")
	if err != nil {
		return value
	}
	return string(formatted)
}

func indent(value string) string {
	if value == "" {
		return "  (empty)"
	}
	return "  " + strings.ReplaceAll(value, "\n", "\n  ")
}

func newShell(configured options) (*gash.Bash, string, error) {
	root, err := filepath.Abs(configured.root)
	if err != nil {
		return nil, "", fmt.Errorf("resolve root: %w", err)
	}
	rooted, err := gashfs.NewRooted(root)
	if err != nil {
		return nil, "", fmt.Errorf("open root: %w", err)
	}

	var filesystem iofs.FS = readOnlyFS{filesystem: rooted}
	if configured.writable {
		filesystem = rooted
	}
	gashOptions := gash.Options{
		FS:           filesystem,
		Cwd:          "/",
		LimitProfile: gash.HardenedProfile,
	}
	if configured.networkAllow != "" {
		policy, err := networkPolicy(configured.networkAllow)
		if err != nil {
			return nil, "", err
		}
		gashOptions.Network = &policy
	}
	shell, err := gash.New(gashOptions)
	if err != nil {
		return nil, "", err
	}
	return shell, root, nil
}

func networkPolicy(value string) (network.Policy, error) {
	policy := network.NewPolicy()
	for _, raw := range strings.Split(value, ",") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		rule := network.AllowOrigin(raw)
		if rule.Scheme == "" || rule.Host == "" {
			return network.Policy{}, fmt.Errorf("invalid network origin %q", raw)
		}
		policy.Rules = append(policy.Rules, rule)
	}
	if len(policy.Rules) == 0 {
		return network.Policy{}, fmt.Errorf("network allowlist is empty")
	}
	return policy, nil
}

func shellHandler(shell *gash.Bash) tools.ToolHandler {
	return func(ctx context.Context, toolCall tools.ToolCall, _ tools.Runtime) (*tools.ToolCallResult, error) {
		var arguments shellArgs
		if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &arguments); err != nil {
			return tools.ResultError("invalid shell arguments: " + err.Error()), nil
		}
		if strings.TrimSpace(arguments.Cmd) == "" {
			return tools.ResultError(`missing required "cmd" argument`), nil
		}

		commandCtx := ctx
		cancel := func() {}
		if arguments.Timeout > 0 {
			commandCtx, cancel = context.WithTimeout(ctx, time.Duration(arguments.Timeout)*time.Second)
		}
		defer cancel()

		result := shell.Exec(commandCtx, arguments.Cmd, gash.ExecOptions{
			Cwd:   arguments.Cwd,
			Stdin: arguments.Stdin,
		})
		encoded, err := json.Marshal(shellOutput{
			Stdout:   result.Stdout,
			Stderr:   result.Stderr,
			ExitCode: result.ExitCode,
		})
		if err != nil {
			return nil, err
		}
		return tools.ResultSuccess(string(encoded)), nil
	}
}
