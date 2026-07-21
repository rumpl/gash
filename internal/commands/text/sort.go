package text

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

func commandSort(_ context.Context, args []string, c *CommandContext) int {
	d, e := readInputs(args, c)
	if e != nil {
		return report(c, "sort", e)
	}
	lines := strings.Split(strings.TrimSuffix(string(d), "\n"), "\n")
	sort.Strings(lines)
	for _, l := range lines {
		fmt.Fprintln(c.Stdout, l)
	}
	return 0
}
