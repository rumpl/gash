package text

import (
	"context"
	"fmt"
	"strings"
)

func commandUniq(_ context.Context, args []string, c *CommandContext) int {
	d, e := readInputs(args, c)
	if e != nil {
		return report(c, "uniq", e)
	}
	last := "\x00"
	for _, l := range strings.Split(strings.TrimSuffix(string(d), "\n"), "\n") {
		if l != last {
			fmt.Fprintln(c.Stdout, l)
			last = l
		}
	}
	return 0
}
