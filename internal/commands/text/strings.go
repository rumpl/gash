package text

import (
	"context"
	"fmt"
	"strconv"
)

func commandStrings(_ context.Context, args []string, c *CommandContext) int {
	minimum := 4
	var files []string
	for i := 0; i < len(args); i++ {
		if (args[i] == "-n" || args[i] == "--bytes") && i+1 < len(args) {
			i++
			minimum, _ = strconv.Atoi(args[i])
		} else {
			files = append(files, args[i])
		}
	}
	data, err := readInputs(files, c)
	if err != nil {
		return report(c, "strings", err)
	}
	run := make([]byte, 0)
	flush := func() {
		if len(run) >= minimum {
			fmt.Fprintln(c.Stdout, string(run))
		}
		run = run[:0]
	}
	for _, b := range data {
		if b >= 32 && b <= 126 {
			run = append(run, b)
		} else {
			flush()
		}
	}
	flush()
	return 0
}
