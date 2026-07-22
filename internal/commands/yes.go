package commands

import (
	"context"
	"fmt"
	"strings"
)

func commandYes(ctx context.Context, args []string, c *CommandContext) int {
	value := "y"
	if len(args) > 0 {
		value = strings.Join(args, " ")
	}
	line := value + "\n"
	for {
		select {
		case <-ctx.Done():
			return 124
		default:
			if _, err := fmt.Fprint(c.Stdout, line); err != nil {
				return 1
			}
		}
	}
}
