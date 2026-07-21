package text

import (
	"context"
	"fmt"
	"strings"
)

func commandWC(_ context.Context, args []string, c *CommandContext) int {
	mode := "l"
	if len(args) > 0 && strings.HasPrefix(args[0], "-") {
		mode = strings.TrimPrefix(args[0], "-")
		args = args[1:]
	}
	d, e := readInputs(args, c)
	if e != nil {
		return report(c, "wc", e)
	}
	var n int
	switch mode {
	case "c":
		n = len(d)
	case "w":
		n = len(strings.Fields(string(d)))
	default:
		n = bytesCount(d, '\n')
	}
	fmt.Fprintf(c.Stdout, "%d\n", n)
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
