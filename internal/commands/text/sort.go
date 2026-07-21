package text

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/rumpl/gash/internal/commandhelp"
	gfs "github.com/rumpl/gash/pkg/fs"
)

func commandSort(_ context.Context, args []string, c *CommandContext) int {
	reverse, numeric, ignoreCase, unique := false, false, false, false
	output := ""
	var files []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-r" || arg == "--reverse":
			reverse = true
		case arg == "-n" || arg == "--numeric-sort":
			numeric = true
		case arg == "-f" || arg == "--ignore-case":
			ignoreCase = true
		case arg == "-u" || arg == "--unique":
			unique = true
		case arg == "-o" && i+1 < len(args):
			i++
			output = args[i]
		case strings.HasPrefix(arg, "--output="):
			output = strings.TrimPrefix(arg, "--output=")
		case strings.HasPrefix(arg, "-") && arg != "-":
			return commandhelp.UnknownOption(c, "sort", arg)
		default:
			files = append(files, arg)
		}
	}
	d, e := readInputs(files, c)
	if e != nil {
		return report(c, "sort", e)
	}
	lines := strings.Split(strings.TrimSuffix(string(d), "\n"), "\n")
	sort.SliceStable(lines, func(i, j int) bool {
		cmp := compareSortLines(lines[i], lines[j], numeric, ignoreCase)
		if reverse {
			return cmp > 0
		}
		return cmp < 0
	})
	if unique {
		seen := map[string]bool{}
		out := lines[:0]
		for _, line := range lines {
			key := line
			if ignoreCase {
				key = strings.ToLower(key)
			}
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, line)
		}
		lines = out
	}
	var builder strings.Builder
	for _, l := range lines {
		fmt.Fprintln(&builder, l)
	}
	if output != "" {
		if err := gfs.WriteFile(c.FS, abs(c, output), []byte(builder.String()), 0o644); err != nil {
			return report(c, "sort", err)
		}
		return 0
	}
	fmt.Fprint(c.Stdout, builder.String())
	return 0
}

func compareSortLines(left, right string, numeric, ignoreCase bool) int {
	cmpLeft, cmpRight := left, right
	if ignoreCase {
		cmpLeft, cmpRight = strings.ToLower(cmpLeft), strings.ToLower(cmpRight)
	}
	if numeric {
		lf, _ := strconv.ParseFloat(firstField(cmpLeft), 64)
		rf, _ := strconv.ParseFloat(firstField(cmpRight), 64)
		switch {
		case lf < rf:
			return -1
		case lf > rf:
			return 1
		}
	}
	switch {
	case cmpLeft < cmpRight:
		return -1
	case cmpLeft > cmpRight:
		return 1
	default:
		return 0
	}
}

func firstField(s string) string {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return "0"
	}
	return fields[0]
}
