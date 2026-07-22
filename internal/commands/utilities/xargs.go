package utilities

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/rumpl/gash/internal/command"
	"github.com/rumpl/gash/internal/commandhelp"
)

const (
	defaultXargsMaxInput = 32 << 20
	defaultXargsMaxItems = 100_000
)

type xargsOptions struct {
	replacement  *string
	delimiter    *string
	maxArgs      int
	maxProcesses int
	null         bool
	verbose      bool
	noRunEmpty   bool
	command      []string
}

type xargsResult struct {
	stdout   string
	stderr   string
	exitCode int
}

func Command() command.Command {
	return command.Command{Name: "xargs", Run: commandXargs}
}

func commandXargs(ctx context.Context, args []string, commandCtx *command.Context) int {
	options, exitCode := parseXargsOptions(args, commandCtx)
	if exitCode != 0 {
		return exitCode
	}
	input, err := readXargsInput(commandCtx.Stdin)
	if err != nil {
		fmt.Fprintln(commandCtx.Stderr, "xargs:", err)
		return 1
	}
	items, err := splitXargsInput(string(input), options)
	if err != nil {
		fmt.Fprintln(commandCtx.Stderr, err)
		return 1
	}
	if len(items) > defaultXargsMaxItems {
		fmt.Fprintf(commandCtx.Stderr, "xargs: array element limit exceeded (%d)\n", defaultXargsMaxItems)
		return 1
	}
	if len(items) == 0 {
		return 0
	}

	invocations, err := buildXargsInvocations(options, items)
	if err != nil {
		fmt.Fprintln(commandCtx.Stderr, err)
		return 1
	}
	results := runXargsInvocations(ctx, commandCtx, invocations, options.maxProcesses, options.verbose)
	finalExitCode := 0
	for _, result := range results {
		fmt.Fprint(commandCtx.Stdout, result.stdout)
		fmt.Fprint(commandCtx.Stderr, result.stderr)
		if result.exitCode != 0 {
			finalExitCode = result.exitCode
		}
	}
	return finalExitCode
}

func parseXargsOptions(args []string, commandCtx *command.Context) (xargsOptions, int) {
	options := xargsOptions{}
	commandStart := len(args)
	for index := 0; index < len(args); index++ {
		arg := args[index]
		switch {
		case arg == "-I" && index+1 < len(args):
			index++
			value := args[index]
			options.replacement = &value
			commandStart = index + 1
		case arg == "-d" && index+1 < len(args):
			index++
			value := decodeXargsDelimiter(args[index])
			options.delimiter = &value
			commandStart = index + 1
		case arg == "-n" && index+1 < len(args):
			index++
			value, err := positiveXargsNumber(args[index])
			if err != nil {
				fmt.Fprintf(commandCtx.Stderr, "xargs: invalid number for -n: '%s'\n", args[index])
				return options, 1
			}
			options.maxArgs = value
			commandStart = index + 1
		case arg == "-P" && index+1 < len(args):
			index++
			value, err := nonnegativeXargsNumber(args[index])
			if err != nil {
				fmt.Fprintf(commandCtx.Stderr, "xargs: invalid number for -P: '%s'\n", args[index])
				return options, 1
			}
			options.maxProcesses = value
			commandStart = index + 1
		case strings.HasPrefix(arg, "-n") && len(arg) > 2:
			value := strings.TrimPrefix(arg, "-n")
			number, err := positiveXargsNumber(value)
			if err != nil {
				fmt.Fprintf(commandCtx.Stderr, "xargs: invalid number for -n: '%s'\n", value)
				return options, 1
			}
			options.maxArgs = number
			commandStart = index + 1
		case strings.HasPrefix(arg, "--max-args="):
			value := strings.TrimPrefix(arg, "--max-args=")
			number, err := positiveXargsNumber(value)
			if err != nil {
				fmt.Fprintf(commandCtx.Stderr, "xargs: invalid number for --max-args: '%s'\n", value)
				return options, 1
			}
			options.maxArgs = number
			commandStart = index + 1
		case strings.HasPrefix(arg, "-P") && len(arg) > 2:
			value := strings.TrimPrefix(arg, "-P")
			number, err := nonnegativeXargsNumber(value)
			if err != nil {
				fmt.Fprintf(commandCtx.Stderr, "xargs: invalid number for -P: '%s'\n", value)
				return options, 1
			}
			options.maxProcesses = number
			commandStart = index + 1
		case strings.HasPrefix(arg, "-I") && len(arg) > 2:
			value := strings.TrimPrefix(arg, "-I")
			options.replacement = &value
			commandStart = index + 1
		case strings.HasPrefix(arg, "-d") && len(arg) > 2:
			value := decodeXargsDelimiter(strings.TrimPrefix(arg, "-d"))
			options.delimiter = &value
			commandStart = index + 1
		case arg == "--":
			commandStart = index + 1
			index = len(args)
		case arg == "-0" || arg == "--null":
			options.null = true
			commandStart = index + 1
		case arg == "-t" || arg == "--verbose":
			options.verbose = true
			commandStart = index + 1
		case arg == "-r" || arg == "--no-run-if-empty":
			options.noRunEmpty = true
			commandStart = index + 1
		case strings.HasPrefix(arg, "--"):
			return options, commandhelp.UnknownOption(commandCtx, "xargs", arg)
		case strings.HasPrefix(arg, "-") && len(arg) > 1:
			for _, option := range strings.TrimPrefix(arg, "-") {
				switch option {
				case '0':
					options.null = true
				case 't':
					options.verbose = true
				case 'r':
					options.noRunEmpty = true
				default:
					return options, commandhelp.UnknownOption(commandCtx, "xargs", "-"+string(option))
				}
			}
			commandStart = index + 1
		default:
			commandStart = index
			index = len(args)
		}
	}
	options.command = append([]string(nil), args[commandStart:]...)
	if len(options.command) == 0 {
		options.command = []string{"echo"}
	}
	return options, 0
}

func positiveXargsNumber(value string) (int, error) {
	number, err := nonnegativeXargsNumber(value)
	if err != nil || number < 1 {
		return 0, fmt.Errorf("not positive")
	}
	return number, nil
}

func nonnegativeXargsNumber(value string) (int, error) {
	if value == "" {
		return 0, fmt.Errorf("empty number")
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return 0, fmt.Errorf("not a number")
		}
	}
	number, err := strconv.ParseUint(value, 10, 31)
	if err != nil {
		return 0, err
	}
	return int(number), nil
}

func decodeXargsDelimiter(value string) string {
	replacer := strings.NewReplacer(
		`\n`, "\n",
		`\t`, "\t",
		`\r`, "\r",
		`\0`, "\x00",
		`\\`, `\`,
	)
	return replacer.Replace(value)
}

func readXargsInput(reader io.Reader) ([]byte, error) {
	limited := io.LimitReader(reader, defaultXargsMaxInput+1)
	input, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if len(input) > defaultXargsMaxInput {
		return nil, fmt.Errorf("input size limit exceeded (%d bytes)", defaultXargsMaxInput)
	}
	if !utf8.Valid(input) {
		input = bytes.ToValidUTF8(input, []byte("�"))
	}
	return input, nil
}

func splitXargsInput(input string, options xargsOptions) ([]string, error) {
	switch {
	case options.null:
		return splitXargsExact(input, "\x00")
	case options.delimiter != nil:
		if *options.delimiter == "" {
			return nil, fmt.Errorf("xargs: delimiter must not be empty")
		}
		input = strings.TrimSuffix(input, "\n")
		return splitXargsExact(input, *options.delimiter)
	default:
		return strings.FieldsFunc(input, unicode.IsSpace), nil
	}
}

func splitXargsExact(input, delimiter string) ([]string, error) {
	if delimiter == "" {
		return nil, fmt.Errorf("xargs: delimiter must not be empty")
	}
	parts := strings.Split(input, delimiter)
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			items = append(items, part)
		}
	}
	return items, nil
}

func buildXargsInvocations(options xargsOptions, items []string) ([][]string, error) {
	if options.replacement != nil {
		if *options.replacement == "" {
			return nil, fmt.Errorf("xargs: replacement string must not be empty")
		}
		invocations := make([][]string, 0, len(items))
		for _, item := range items {
			argv := make([]string, len(options.command))
			for index, value := range options.command {
				argv[index] = strings.ReplaceAll(value, *options.replacement, item)
			}
			invocations = append(invocations, argv)
		}
		return invocations, nil
	}
	if options.maxArgs > 0 {
		invocations := make([][]string, 0, (len(items)+options.maxArgs-1)/options.maxArgs)
		for start := 0; start < len(items); start += options.maxArgs {
			end := min(start+options.maxArgs, len(items))
			argv := append([]string(nil), options.command...)
			argv = append(argv, items[start:end]...)
			invocations = append(invocations, argv)
		}
		return invocations, nil
	}
	argv := append([]string(nil), options.command...)
	argv = append(argv, items...)
	return [][]string{argv}, nil
}

func runXargsInvocations(ctx context.Context, commandCtx *command.Context, invocations [][]string, maxProcesses int, verbose bool) []xargsResult {
	results := make([]xargsResult, len(invocations))
	if maxProcesses <= 1 {
		for index, argv := range invocations {
			results[index] = runXargsInvocation(ctx, commandCtx, argv, verbose)
		}
		return results
	}

	for start := 0; start < len(invocations); start += maxProcesses {
		end := min(start+maxProcesses, len(invocations))
		var wait sync.WaitGroup
		for index := start; index < end; index++ {
			wait.Add(1)
			go func() {
				defer wait.Done()
				results[index] = runXargsInvocation(ctx, commandCtx, invocations[index], verbose)
			}()
		}
		wait.Wait()
	}
	return results
}

func runXargsInvocation(ctx context.Context, commandCtx *command.Context, argv []string, verbose bool) xargsResult {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if verbose {
		fmt.Fprintln(&stderr, quoteXargsCommand(argv))
	}
	if commandCtx.RunCommand == nil {
		fmt.Fprintln(&stdout, quoteXargsCommand(argv))
		return xargsResult{stdout: stdout.String(), stderr: stderr.String()}
	}
	child := *commandCtx
	child.Stdin = strings.NewReader("")
	child.Stdout = &stdout
	child.Stderr = &stderr
	exitCode := commandCtx.RunCommand(ctx, argv, &child)
	return xargsResult{stdout: stdout.String(), stderr: stderr.String(), exitCode: exitCode}
}

func quoteXargsCommand(argv []string) string {
	quoted := make([]string, len(argv))
	for index, value := range argv {
		quoted[index] = quoteXargsArgument(value)
	}
	return strings.Join(quoted, " ")
}

func quoteXargsArgument(value string) string {
	if !strings.ContainsAny(value, " \t\r\n\"'\\$`!*?[]{}();&|<>#") {
		return value
	}
	replacer := strings.NewReplacer(`\`, `\\`, `"`, `\"`, `$`, `\$`, "`", "\\`")
	return `"` + replacer.Replace(value) + `"`
}
