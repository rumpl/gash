package text

import (
	"context"
	"fmt"
	"strings"

	"github.com/rumpl/gash/internal/commandhelp"
)

type uniqOptions struct {
	count      bool
	repeated   bool
	unique     bool
	ignoreCase bool
	positional []string
}

func commandUniq(_ context.Context, args []string, c *CommandContext) int {
	options, exitCode := parseUniqOptions(args, c)
	if exitCode != 0 {
		return exitCode
	}
	data, err := readInputs(options.positional, c)
	if err != nil {
		return report(c, "uniq", err)
	}
	if len(data) == 0 {
		return 0
	}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	for start := 0; start < len(lines); {
		end := start + 1
		for end < len(lines) && uniqLinesEqual(lines[start], lines[end], options.ignoreCase) {
			end++
		}
		count := end - start
		include := true
		if options.repeated {
			include = count > 1
		} else if options.unique {
			include = count == 1
		}
		if include {
			if options.count {
				fmt.Fprintf(c.Stdout, "%4d %s\n", count, lines[start])
			} else {
				fmt.Fprintln(c.Stdout, lines[start])
			}
		}
		start = end
	}
	return 0
}

func parseUniqOptions(args []string, c *CommandContext) (uniqOptions, int) {
	var options uniqOptions
	parseOptions := true
	for _, arg := range args {
		if parseOptions && arg == "--" {
			parseOptions = false
			continue
		}
		if parseOptions && strings.HasPrefix(arg, "--") {
			switch arg {
			case "--count":
				options.count = true
			case "--repeated":
				options.repeated = true
			case "--unique":
				options.unique = true
			case "--ignore-case":
				options.ignoreCase = true
			default:
				return options, commandhelp.UnknownOption(c, "uniq", arg)
			}
			continue
		}
		if parseOptions && strings.HasPrefix(arg, "-") && arg != "-" {
			for _, option := range strings.TrimPrefix(arg, "-") {
				switch option {
				case 'c':
					options.count = true
				case 'd':
					options.repeated = true
				case 'u':
					options.unique = true
				case 'i':
					options.ignoreCase = true
				default:
					return options, commandhelp.UnknownOption(c, "uniq", "-"+string(option))
				}
			}
			continue
		}
		parseOptions = false
		options.positional = append(options.positional, arg)
	}
	return options, 0
}

func uniqLinesEqual(left, right string, ignoreCase bool) bool {
	if ignoreCase {
		return strings.EqualFold(left, right)
	}
	return left == right
}
