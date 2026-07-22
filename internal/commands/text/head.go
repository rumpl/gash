package text

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/rumpl/gash/internal/commandhelp"
)

func allDecimalDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, digit := range value {
		if digit < '0' || digit > '9' {
			return false
		}
	}
	return true
}

func commandHead(_ context.Context, args []string, c *CommandContext) int {
	n := 10
	bytesMode := false
	var files []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-n" && i+1 < len(args):
			i++
			n, _ = strconv.Atoi(args[i])
		case strings.HasPrefix(arg, "-n") && len(arg) > 2:
			n, _ = strconv.Atoi(arg[2:])
		case len(arg) > 1 && arg[0] == '-' && allDecimalDigits(arg[1:]):
			n, _ = strconv.Atoi(arg[1:])
		case strings.HasPrefix(arg, "--lines="):
			n, _ = strconv.Atoi(strings.TrimPrefix(arg, "--lines="))
		case arg == "-c" && i+1 < len(args):
			i++
			bytesMode = true
			n, _ = strconv.Atoi(args[i])
		case strings.HasPrefix(arg, "-c") && len(arg) > 2:
			bytesMode = true
			n, _ = strconv.Atoi(arg[2:])
		case strings.HasPrefix(arg, "--bytes="):
			bytesMode = true
			n, _ = strconv.Atoi(strings.TrimPrefix(arg, "--bytes="))
		case arg == "-q" || arg == "--quiet" || arg == "-v" || arg == "--verbose":
			// Headers for multi-file output are intentionally deferred; gash concatenates inputs like just-bash's simpler common cases.
		case strings.HasPrefix(arg, "-") && arg != "-":
			return commandhelp.UnknownOption(c, "head", arg)
		default:
			files = append(files, arg)
		}
	}
	chunks, e := readInputChunks(files, c)
	if e != nil {
		return report(c, "head", e)
	}
	for _, d := range chunks {
		if n < 0 {
			n = 0
		}
		if bytesMode {
			if n < len(d) {
				d = d[:n]
			}
			c.Stdout.Write(d)
			continue
		}
		lines := strings.SplitAfter(string(d), "\n")
		if n < len(lines) {
			lines = lines[:n]
		}
		fmt.Fprint(c.Stdout, strings.Join(lines, ""))
	}
	return 0
}
