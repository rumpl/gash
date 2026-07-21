package text

import (
	"bufio"
	"context"
	"fmt"
	"strconv"
	"strings"
)

func commandNL(_ context.Context, args []string, c *CommandContext) int {
	width := 6
	separator := "\t"
	numberBlank := false
	var files []string
	for i := 0; i < len(args); i++ {
		if args[i] == "-ba" {
			numberBlank = true
		} else if args[i] == "-w" && i+1 < len(args) {
			i++
			width, _ = strconv.Atoi(args[i])
		} else if args[i] == "-s" && i+1 < len(args) {
			i++
			separator = args[i]
		} else {
			files = append(files, args[i])
		}
	}
	data, err := readInputs(files, c)
	if err != nil {
		return report(c, "nl", err)
	}
	n := 1
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := scanner.Text()
		if line != "" || numberBlank {
			fmt.Fprintf(c.Stdout, "%*d%s%s\n", width, n, separator, line)
			n++
		} else {
			fmt.Fprintln(c.Stdout, line)
		}
	}
	return 0
}
