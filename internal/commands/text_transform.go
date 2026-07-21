package commands

import (
	"bufio"
	"context"
	"fmt"
	"strconv"
	"strings"
)

func commandRev(_ context.Context, args []string, c *CommandContext) int {
	data, err := readInputs(args, c)
	if err != nil {
		return report(c, "rev", err)
	}
	for _, line := range strings.SplitAfter(string(data), "\n") {
		newline := strings.HasSuffix(line, "\n")
		line = strings.TrimSuffix(line, "\n")
		r := []rune(line)
		for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
			r[i], r[j] = r[j], r[i]
		}
		fmt.Fprint(c.Stdout, string(r))
		if newline {
			fmt.Fprintln(c.Stdout)
		}
	}
	return 0
}

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

func commandTr(_ context.Context, args []string, c *CommandContext) int {
	del, squeeze := false, false
	var sets []string
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			del = del || strings.Contains(arg, "d")
			squeeze = squeeze || strings.Contains(arg, "s")
		} else {
			sets = append(sets, arg)
		}
	}
	if len(sets) == 0 {
		return report(c, "tr", fmt.Errorf("missing operand"))
	}
	from := expandSet(sets[0])
	to := []rune{}
	if len(sets) > 1 {
		to = expandSet(sets[1])
	}
	mapping := map[rune]rune{}
	for i, r := range from {
		if len(to) > 0 {
			j := i
			if j >= len(to) {
				j = len(to) - 1
			}
			mapping[r] = to[j]
		}
	}
	input, _ := ioReadAll(c)
	var out strings.Builder
	var last rune
	hasLast := false
	for _, r := range []rune(input) {
		_, match := mapping[r]
		if del {
			match = strings.ContainsRune(string(from), r)
			if match {
				continue
			}
		} else if match {
			r = mapping[r]
		}
		if squeeze && hasLast && r == last && (len(to) == 0 || strings.ContainsRune(string(to), r)) {
			continue
		}
		out.WriteRune(r)
		last = r
		hasLast = true
	}
	fmt.Fprint(c.Stdout, out.String())
	return 0
}

func expandSet(s string) []rune {
	r := []rune(s)
	var out []rune
	for i := 0; i < len(r); i++ {
		if i+2 < len(r) && r[i+1] == '-' && r[i] <= r[i+2] {
			for x := r[i]; x <= r[i+2]; x++ {
				out = append(out, x)
			}
			i += 2
		} else if r[i] == '\\' && i+1 < len(r) {
			i++
			switch r[i] {
			case 'n':
				out = append(out, '\n')
			case 't':
				out = append(out, '\t')
			default:
				out = append(out, r[i])
			}
		} else {
			out = append(out, r[i])
		}
	}
	return out
}

func ioReadAll(c *CommandContext) (string, error) {
	var b strings.Builder
	_, err := bufio.NewReader(c.Stdin).WriteTo(&b)
	return b.String(), err
}

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
