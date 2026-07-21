package text

import (
	"context"
	"fmt"
	"strings"
)

func commandRev(_ context.Context, args []string, c *CommandContext) int {
	data, err := readInputs(args, c)
	if err != nil {
		return report(c, "rev", err)
	}
	for _, line := range strings.SplitAfter(string(data), "\n") {
		newline := strings.HasSuffix(line, "\n")
		line = strings.TrimSuffix(line, "\n")
		r := []rune(line)
		for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
			r[i], r[j] = r[j], r[i]
		}
		fmt.Fprint(c.Stdout, string(r))
		if newline {
			fmt.Fprintln(c.Stdout)
		}
	}
	return 0
}
