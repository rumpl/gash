package text

import (
	"context"
	"fmt"
	"strings"
)

func commandTac(_ context.Context, args []string, c *CommandContext) int {
	data, err := readInputs(args, c)
	if err != nil {
		return report(c, "tac", err)
	}
	text := string(data)
	trailing := strings.HasSuffix(text, "\n")
	lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		fmt.Fprint(c.Stdout, lines[i])
		if i > 0 || trailing {
			fmt.Fprintln(c.Stdout)
		}
	}
	return 0
}
