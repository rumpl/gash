package text

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"
)

func commandColumn(_ context.Context, args []string, c *CommandContext) int {
	separator := ""
	table := false
	var files []string
	for i := 0; i < len(args); i++ {
		if args[i] == "-t" {
			table = true
		} else if args[i] == "-s" && i+1 < len(args) {
			i++
			separator = args[i]
		} else {
			files = append(files, args[i])
		}
	}
	data, err := readInputs(files, c)
	if err != nil {
		return report(c, "column", err)
	}
	if !table {
		fmt.Fprint(c.Stdout, string(data))
		return 0
	}
	var rows [][]string
	var widths []int
	for _, line := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
		fields := strings.Fields(line)
		if separator != "" {
			fields = strings.Split(line, separator)
		}
		rows = append(rows, fields)
		for i, v := range fields {
			if i >= len(widths) {
				widths = append(widths, 0)
			}
			if utf8.RuneCountInString(v) > widths[i] {
				widths[i] = utf8.RuneCountInString(v)
			}
		}
	}
	for _, row := range rows {
		for i, v := range row {
			if i == len(row)-1 {
				fmt.Fprint(c.Stdout, v)
			} else {
				fmt.Fprintf(c.Stdout, "%-*s  ", widths[i], v)
			}
		}
		fmt.Fprintln(c.Stdout)
	}
	return 0
}
