package text

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

func commandHead(_ context.Context, args []string, c *CommandContext) int {
	n := 10
	if len(args) >= 2 && args[0] == "-n" {
		n, _ = strconv.Atoi(args[1])
		args = args[2:]
	}
	d, e := readInputs(args, c)
	if e != nil {
		return report(c, "head", e)
	}
	lines := strings.SplitAfter(string(d), "\n")
	if n < len(lines) {
		lines = lines[:n]
	}
	fmt.Fprint(c.Stdout, strings.Join(lines, ""))
	return 0
}
