package text

import (
	"context"
	"fmt"
	"strings"
)

func commandOD(_ context.Context, args []string, c *CommandContext) int {
	format := "o"
	var files []string
	for _, arg := range args {
		if arg == "-x" {
			format = "x"
		} else if arg == "-c" {
			format = "c"
		} else if strings.HasPrefix(arg, "-") {
			continue
		} else {
			files = append(files, arg)
		}
	}
	data, err := readInputs(files, c)
	if err != nil {
		return report(c, "od", err)
	}
	for offset := 0; offset < len(data); offset += 16 {
		end := offset + 16
		if end > len(data) {
			end = len(data)
		}
		fmt.Fprintf(c.Stdout, "%07o", offset)
		for _, b := range data[offset:end] {
			switch format {
			case "x":
				fmt.Fprintf(c.Stdout, " %02x", b)
			case "c":
				if b >= 32 && b < 127 {
					fmt.Fprintf(c.Stdout, "   %c", b)
				} else {
					fmt.Fprintf(c.Stdout, " %03o", b)
				}
			default:
				fmt.Fprintf(c.Stdout, " %03o", b)
			}
		}
		fmt.Fprintln(c.Stdout)
	}
	fmt.Fprintf(c.Stdout, "%07o\n", len(data))
	return 0
}
