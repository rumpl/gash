package text

import (
	"bufio"
	"context"
	"fmt"
	"strconv"
	"strings"
)

func commandCut(_ context.Context, args []string, c *CommandContext) int {
	delimiter := "\t"
	mode := ""
	selection := ""
	onlyDelimited := false
	var files []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-d" && i+1 < len(args):
			i++
			delimiter = args[i]
		case strings.HasPrefix(arg, "-d") && len(arg) > 2:
			delimiter = arg[2:]
		case (arg == "-f" || arg == "-c" || arg == "-b") && i+1 < len(args):
			mode = arg[1:]
			i++
			selection = args[i]
		case len(arg) > 2 && (strings.HasPrefix(arg, "-f") || strings.HasPrefix(arg, "-c") || strings.HasPrefix(arg, "-b")):
			mode = arg[1:2]
			selection = arg[2:]
		case arg == "-s":
			onlyDelimited = true
		default:
			files = append(files, arg)
		}
	}
	if mode == "" {
		return report(c, "cut", fmt.Errorf("you must specify a list of bytes, characters, or fields"))
	}
	indices, err := parseSelection(selection)
	if err != nil {
		return report(c, "cut", err)
	}
	data, err := readInputs(files, c)
	if err != nil {
		return report(c, "cut", err)
	}
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := scanner.Text()
		if mode == "f" {
			if !strings.Contains(line, delimiter) {
				if !onlyDelimited {
					fmt.Fprintln(c.Stdout, line)
				}
				continue
			}
			parts := strings.Split(line, delimiter)
			fmt.Fprintln(c.Stdout, selectStrings(parts, indices, delimiter))
		} else {
			if mode == "b" {
				var selected []byte
				for _, i := range indices {
					if i > 0 && i <= len(line) {
						selected = append(selected, line[i-1])
					}
				}
				fmt.Fprintln(c.Stdout, string(selected))
				continue
			}
			r := []rune(line)
			var selected []rune
			for _, i := range indices {
				if i > 0 && i <= len(r) {
					selected = append(selected, r[i-1])
				}
			}
			fmt.Fprintln(c.Stdout, string(selected))
		}
	}
	return 0
}

func parseSelection(s string) ([]int, error) {
	seen := map[int]bool{}
	var out []int
	for _, part := range strings.Split(s, ",") {
		if strings.Contains(part, "-") {
			p := strings.SplitN(part, "-", 2)
			start, end := 1, 0
			if p[0] != "" {
				start, _ = strconv.Atoi(p[0])
			}
			if p[1] != "" {
				end, _ = strconv.Atoi(p[1])
			} else {
				end = start + 100000
			}
			if start < 1 || end < start {
				return nil, fmt.Errorf("invalid range")
			}
			for i := start; i <= end && i <= 100000; i++ {
				if !seen[i] {
					out = append(out, i)
					seen[i] = true
				}
			}
		} else {
			n, err := strconv.Atoi(part)
			if err != nil || n < 1 {
				return nil, fmt.Errorf("invalid field value")
			}
			if !seen[n] {
				out = append(out, n)
				seen[n] = true
			}
		}
	}
	return out, nil
}

func selectStrings(parts []string, indices []int, separator string) string {
	var out []string
	for _, i := range indices {
		if i > 0 && i <= len(parts) {
			out = append(out, parts[i-1])
		}
	}
	return strings.Join(out, separator)
}
