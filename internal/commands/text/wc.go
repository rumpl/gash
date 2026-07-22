package text

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/rumpl/gash/internal/commandhelp"
)

func commandWC(_ context.Context, args []string, c *CommandContext) int {
	mode := ""
	var files []string
	for _, arg := range args {
		switch {
		case arg == "-c" || arg == "--bytes":
			mode = "c"
		case arg == "-m" || arg == "--chars":
			mode = "m"
		case arg == "-l" || arg == "--lines":
			mode = "l"
		case arg == "-w" || arg == "--words":
			mode = "w"
		case strings.HasPrefix(arg, "-") && arg != "-":
			flags := strings.TrimPrefix(arg, "-")
			for _, flag := range flags {
				switch flag {
				case 'c', 'm', 'l', 'w':
					mode = string(flag)
				default:
					return commandhelp.UnknownOption(c, "wc", "-"+string(flag))
				}
			}
		default:
			files = append(files, arg)
		}
	}
	d, e := readInputs(files, c)
	if e != nil {
		return report(c, "wc", e)
	}
	lines := bytesCount(d, '\n')
	words := len(strings.Fields(string(d)))
	bytesN := len(d)
	chars := utf8.RuneCount(d)
	switch mode {
	case "c":
		fmt.Fprintf(c.Stdout, "%d\n", bytesN)
	case "m":
		fmt.Fprintf(c.Stdout, "%d\n", chars)
	case "l":
		fmt.Fprintf(c.Stdout, "%d\n", lines)
	case "w":
		fmt.Fprintf(c.Stdout, "%d\n", words)
	default:
		fmt.Fprintf(c.Stdout, "%d %d %d\n", lines, words, bytesN)
	}
	return 0
}

func bytesCount(d []byte, b byte) int {
	n := 0
	for _, v := range d {
		if v == b {
			n++
		}
	}
	return n
}
