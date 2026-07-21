package text

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

func commandExpand(_ context.Context, args []string, c *CommandContext) int {
	return expandTabs(args, c, false)
}

func expandTabs(args []string, c *CommandContext, reverse bool) int {
	width := 8
	all := false
	var files []string
	for i := 0; i < len(args); i++ {
		if args[i] == "-a" {
			all = true
		} else if args[i] == "-t" && i+1 < len(args) {
			i++
			width, _ = strconv.Atoi(strings.Split(args[i], ",")[0])
		} else {
			files = append(files, args[i])
		}
	}
	data, err := readInputs(files, c)
	if err != nil {
		return report(c, "expand", err)
	}
	if !reverse {
		column := 0
		for _, r := range string(data) {
			if r == '\n' {
				fmt.Fprint(c.Stdout, "\n")
				column = 0
			} else if r == '\t' {
				n := width - column%width
				fmt.Fprint(c.Stdout, strings.Repeat(" ", n))
				column += n
			} else {
				fmt.Fprint(c.Stdout, string(r))
				column++
			}
		}
		return 0
	}
	for _, line := range strings.SplitAfter(string(data), "\n") {
		body := strings.TrimSuffix(line, "\n")
		limit := len(body)
		if !all {
			limit = 0
			for limit < len(body) && body[limit] == ' ' {
				limit++
			}
		}
		prefix := body[:limit]
		for strings.Contains(prefix, strings.Repeat(" ", width)) {
			prefix = strings.Replace(prefix, strings.Repeat(" ", width), "\t", 1)
		}
		fmt.Fprint(c.Stdout, prefix+body[limit:])
		if strings.HasSuffix(line, "\n") {
			fmt.Fprintln(c.Stdout)
		}
	}
	return 0
}
