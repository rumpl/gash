package text

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

func commandFold(_ context.Context, args []string, c *CommandContext) int {
	width := 80
	spaces := false
	var files []string
	for i := 0; i < len(args); i++ {
		if args[i] == "-w" && i+1 < len(args) {
			i++
			width, _ = strconv.Atoi(args[i])
		} else if strings.HasPrefix(args[i], "-w") && len(args[i]) > 2 {
			width, _ = strconv.Atoi(args[i][2:])
		} else if args[i] == "-s" {
			spaces = true
		} else {
			files = append(files, args[i])
		}
	}
	if width <= 0 {
		return report(c, "fold", fmt.Errorf("invalid width"))
	}
	data, err := readInputs(files, c)
	if err != nil {
		return report(c, "fold", err)
	}
	for _, line := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
		r := []rune(line)
		for len(r) > width {
			cut := width
			if spaces {
				for i := width; i > 0; i-- {
					if r[i-1] == ' ' || r[i-1] == '\t' {
						cut = i
						break
					}
				}
			}
			fmt.Fprintln(c.Stdout, string(r[:cut]))
			r = r[cut:]
			for spaces && len(r) > 0 && r[0] == ' ' {
				r = r[1:]
			}
		}
		fmt.Fprintln(c.Stdout, string(r))
	}
	return 0
}
