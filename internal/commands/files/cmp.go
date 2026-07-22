package files

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/rumpl/gash/internal/commandutil"
	gfs "github.com/rumpl/gash/pkg/fs"
)

type cmpOptions struct {
	silent  bool
	verbose bool
	limit   int64
	skip1   int64
	skip2   int64
}

func commandCmp(_ context.Context, args []string, c *CommandContext) int {
	options, files, ok := parseCmpArgs(args, c)
	if !ok {
		return 2
	}
	if len(files) > 2 {
		if len(files) > 4 {
			fmt.Fprintln(c.Stderr, "cmp: too many operands")
			return 2
		}
		first, err := parseCmpCount(files[2])
		if err != nil {
			fmt.Fprintf(c.Stderr, "cmp: invalid skip value %q\n", files[2])
			return 2
		}
		options.skip1 = first
		options.skip2 = 0
		if len(files) == 4 {
			second, secondErr := parseCmpCount(files[3])
			if secondErr != nil {
				fmt.Fprintf(c.Stderr, "cmp: invalid skip value %q\n", files[3])
				return 2
			}
			options.skip2 = second
		}
		files = files[:2]
	}
	if len(files) < 1 {
		fmt.Fprintln(c.Stderr, "cmp: missing operand")
		return 2
	}
	if len(files) == 1 {
		files = append(files, "-")
	}
	if files[0] == "-" && files[1] == "-" {
		fmt.Fprintln(c.Stderr, "cmp: standard input may only be used once")
		return 2
	}
	left, err := readCmpOperand(files[0], c)
	if err != nil {
		fmt.Fprintf(c.Stderr, "cmp: %s: %s\n", files[0], commandutil.ErrorText(err))
		return 2
	}
	right, err := readCmpOperand(files[1], c)
	if err != nil {
		fmt.Fprintf(c.Stderr, "cmp: %s: %s\n", files[1], commandutil.ErrorText(err))
		return 2
	}
	left = skipCmpBytes(left, options.skip1)
	right = skipCmpBytes(right, options.skip2)
	return compareCmpData(left, right, files, options, c)
}

func parseCmpArgs(args []string, c *CommandContext) (cmpOptions, []string, bool) {
	options := cmpOptions{limit: -1}
	files := make([]string, 0, 2)
	for index := 0; index < len(args); index++ {
		argument := args[index]
		switch {
		case argument == "--":
			files = append(files, args[index+1:]...)
			return options, files, true
		case argument == "-s" || argument == "--silent" || argument == "--quiet":
			options.silent = true
		case argument == "-l" || argument == "--verbose":
			options.verbose = true
		case argument == "-n" || argument == "--bytes":
			if index+1 >= len(args) {
				fmt.Fprintf(c.Stderr, "cmp: option %s requires an argument\n", argument)
				return options, nil, false
			}
			index++
			value, err := parseCmpCount(args[index])
			if err != nil {
				fmt.Fprintf(c.Stderr, "cmp: invalid byte count %q\n", args[index])
				return options, nil, false
			}
			options.limit = value
		case strings.HasPrefix(argument, "--bytes="):
			value, err := parseCmpCount(strings.TrimPrefix(argument, "--bytes="))
			if err != nil {
				fmt.Fprintf(c.Stderr, "cmp: invalid byte count %q\n", strings.TrimPrefix(argument, "--bytes="))
				return options, nil, false
			}
			options.limit = value
		case argument == "-i" || argument == "--ignore-initial":
			if index+1 >= len(args) {
				fmt.Fprintf(c.Stderr, "cmp: option %s requires an argument\n", argument)
				return options, nil, false
			}
			index++
			if !parseCmpSkips(args[index], &options) {
				fmt.Fprintf(c.Stderr, "cmp: invalid skip value %q\n", args[index])
				return options, nil, false
			}
		case strings.HasPrefix(argument, "--ignore-initial="):
			value := strings.TrimPrefix(argument, "--ignore-initial=")
			if !parseCmpSkips(value, &options) {
				fmt.Fprintf(c.Stderr, "cmp: invalid skip value %q\n", value)
				return options, nil, false
			}
		case strings.HasPrefix(argument, "-") && argument != "-":
			fmt.Fprintf(c.Stderr, "cmp: unrecognized option %q\n", argument)
			return options, nil, false
		default:
			files = append(files, argument)
		}
	}
	return options, files, true
}

func parseCmpCount(value string) (int64, error) {
	count, err := strconv.ParseInt(value, 10, 64)
	if err != nil || count < 0 {
		return 0, fmt.Errorf("invalid count")
	}
	return count, nil
}

func parseCmpSkips(value string, options *cmpOptions) bool {
	parts := strings.SplitN(value, ":", 2)
	first, err := parseCmpCount(parts[0])
	if err != nil {
		return false
	}
	options.skip1, options.skip2 = first, first
	if len(parts) == 2 {
		second, secondErr := parseCmpCount(parts[1])
		if secondErr != nil {
			return false
		}
		options.skip2 = second
	}
	return true
}

func readCmpOperand(name string, c *CommandContext) ([]byte, error) {
	if name == "-" {
		return io.ReadAll(c.Stdin)
	}
	return gfs.ReadFile(c.FS, abs(c, name))
}

func skipCmpBytes(data []byte, count int64) []byte {
	if count >= int64(len(data)) {
		return nil
	}
	return data[count:]
}

func compareCmpData(left, right []byte, names []string, options cmpOptions, c *CommandContext) int {
	compared := len(left)
	if len(right) < compared {
		compared = len(right)
	}
	if options.limit >= 0 && options.limit < int64(compared) {
		compared = int(options.limit)
	}
	different := false
	line := 1
	for index := 0; index < compared; index++ {
		if left[index] != right[index] {
			different = true
			if !options.silent {
				if options.verbose {
					fmt.Fprintf(c.Stdout, "%d %o %o\n", index+1, left[index], right[index])
				} else {
					fmt.Fprintf(c.Stdout, "%s %s differ: byte %d, line %d\n", names[0], names[1], index+1, line)
					return 1
				}
			}
		}
		if left[index] == '\n' {
			line++
		}
	}
	limitReached := options.limit >= 0 && int64(compared) >= options.limit
	if !limitReached && len(left) != len(right) {
		different = true
		if !options.silent && !options.verbose {
			shorter := names[0]
			if len(right) < len(left) {
				shorter = names[1]
			}
			fmt.Fprintf(c.Stderr, "cmp: EOF on %s after byte %d, line %d\n", shorter, compared, line)
		}
	}
	if different {
		return 1
	}
	return 0
}
