package text

import (
	"bufio"
	"context"
	"fmt"
	"regexp"
	"strings"
)

func commandGrep(_ context.Context, args []string, c *CommandContext) int {
	ignore, invert, number := false, false, false
	var rest []string
	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			ignore = ignore || strings.Contains(a, "i")
			invert = invert || strings.Contains(a, "v")
			number = number || strings.Contains(a, "n")
		} else {
			rest = append(rest, a)
		}
	}
	if len(rest) == 0 {
		return 2
	}
	pattern := rest[0]
	if ignore {
		pattern = "(?i)" + pattern
	}
	re, e := regexp.Compile(pattern)
	if e != nil {
		return report(c, "grep", e)
	}
	d, e := readInputs(rest[1:], c)
	if e != nil {
		return report(c, "grep", e)
	}
	found := false
	scan := bufio.NewScanner(strings.NewReader(string(d)))
	line := 0
	for scan.Scan() {
		line++
		match := re.MatchString(scan.Text())
		if invert {
			match = !match
		}
		if match {
			found = true
			if number {
				fmt.Fprintf(c.Stdout, "%d:", line)
			}
			fmt.Fprintln(c.Stdout, scan.Text())
		}
	}
	if found {
		return 0
	}
	return 1
}
