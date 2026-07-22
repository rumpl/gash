package text

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/rumpl/gash/internal/commandhelp"
)

func commandTail(_ context.Context, args []string, c *CommandContext) int {
	n := 10
	bytesMode := false
	fromStart := false
	var files []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-n" && i+1 < len(args):
			i++
			n, fromStart = parseTailCount(args[i])
		case strings.HasPrefix(arg, "-n") && len(arg) > 2:
			n, fromStart = parseTailCount(arg[2:])
		case strings.HasPrefix(arg, "--lines="):
			n, fromStart = parseTailCount(strings.TrimPrefix(arg, "--lines="))
		case arg == "-c" && i+1 < len(args):
			i++
			bytesMode = true
			n, fromStart = parseTailCount(args[i])
		case strings.HasPrefix(arg, "-c") && len(arg) > 2:
			bytesMode = true
			n, fromStart = parseTailCount(arg[2:])
		case strings.HasPrefix(arg, "--bytes="):
			bytesMode = true
			n, fromStart = parseTailCount(strings.TrimPrefix(arg, "--bytes="))
		case arg == "-q" || arg == "--quiet" || arg == "-v" || arg == "--verbose":
			// Headers for multi-file output are intentionally deferred; gash concatenates inputs like just-bash's simpler common cases.
		case strings.HasPrefix(arg, "-") && arg != "-":
			return commandhelp.UnknownOption(c, "tail", arg)
		default:
			files = append(files, arg)
		}
	}
	chunks, e := readInputChunks(files, c)
	if e != nil {
		return report(c, "tail", e)
	}
	if n < 0 {
		n = 0
	}
	for _, d := range chunks {
		if bytesMode {
			if fromStart {
				start := n - 1
				if start < 0 {
					start = 0
				}
				if start > len(d) {
					start = len(d)
				}
				c.Stdout.Write(d[start:])
				continue
			}
			if n < len(d) {
				d = d[len(d)-n:]
			}
			c.Stdout.Write(d)
			continue
		}
		lines := strings.SplitAfter(string(d), "\n")
		if len(lines) > 0 && lines[len(lines)-1] == "" {
			lines = lines[:len(lines)-1]
		}
		if fromStart {
			start := n - 1
			if start < 0 {
				start = 0
			}
			if start > len(lines) {
				start = len(lines)
			}
			lines = lines[start:]
		} else if n < len(lines) {
			lines = lines[len(lines)-n:]
		}
		fmt.Fprint(c.Stdout, strings.Join(lines, ""))
	}
	return 0
}

func parseTailCount(s string) (int, bool) {
	fromStart := strings.HasPrefix(s, "+")
	s = strings.TrimPrefix(s, "+")
	n, _ := strconv.Atoi(s)
	return n, fromStart
}
