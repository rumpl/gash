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

type sortKey struct {
	field   int
	numeric bool
}

func commandSort(_ context.Context, args []string, c *CommandContext) int {
	reverse, numeric, ignoreCase, unique := false, false, false, false
	output, separator := "", ""
	var key *sortKey
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
		case (arg == "-t" || arg == "--field-separator") && i+1 < len(args):
			i++
			separator = args[i]
		case strings.HasPrefix(arg, "-t") && len(arg) > 2:
			separator = strings.TrimPrefix(arg, "-t")
		case strings.HasPrefix(arg, "--field-separator="):
			separator = strings.TrimPrefix(arg, "--field-separator=")
		case (arg == "-k" || arg == "--key") && i+1 < len(args):
			i++
			parsed, err := parseSortKey(args[i])
			if err != nil {
				fmt.Fprintf(c.Stderr, "sort: invalid key %q\n", args[i])
				return 1
			}
			key = &parsed
		case strings.HasPrefix(arg, "-k") && len(arg) > 2:
			parsed, err := parseSortKey(strings.TrimPrefix(arg, "-k"))
			if err != nil {
				fmt.Fprintf(c.Stderr, "sort: invalid key %q\n", strings.TrimPrefix(arg, "-k"))
				return 1
			}
			key = &parsed
		case strings.HasPrefix(arg, "--key="):
			parsed, err := parseSortKey(strings.TrimPrefix(arg, "--key="))
			if err != nil {
				fmt.Fprintf(c.Stderr, "sort: invalid key %q\n", strings.TrimPrefix(arg, "--key="))
				return 1
			}
			key = &parsed
		case strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "--") && arg != "-":
			for _, option := range strings.TrimPrefix(arg, "-") {
				switch option {
				case 'r':
					reverse = true
				case 'n':
					numeric = true
				case 'f':
					ignoreCase = true
				case 'u':
					unique = true
				default:
					return commandhelp.UnknownOption(c, "sort", "-"+string(option))
				}
			}
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
		cmp := compareSortLinesByKey(lines[i], lines[j], numeric, ignoreCase, separator, key)
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
		if err := gfs.WriteFile(c.FS, abs(c, output), []byte(builder.String()), c.CreationMode(0o666)); err != nil {
			return report(c, "sort", err)
		}
		return 0
	}
	fmt.Fprint(c.Stdout, builder.String())
	return 0
}

func parseSortKey(specification string) (sortKey, error) {
	numeric := strings.ContainsRune(specification, 'n')
	start := strings.SplitN(specification, ",", 2)[0]
	start = strings.TrimRightFunc(start, func(character rune) bool {
		return character < '0' || character > '9'
	})
	field, err := strconv.Atoi(start)
	if err != nil || field < 1 {
		return sortKey{}, fmt.Errorf("invalid key")
	}
	return sortKey{field: field, numeric: numeric}, nil
}

func compareSortLinesByKey(left, right string, numeric, ignoreCase bool, separator string, key *sortKey) int {
	if key != nil {
		leftKey := sortField(left, separator, key.field)
		rightKey := sortField(right, separator, key.field)
		comparison := compareSortLines(leftKey, rightKey, numeric || key.numeric, ignoreCase)
		if comparison != 0 {
			return comparison
		}
	}
	return compareSortLines(left, right, numeric && key == nil, ignoreCase)
}

func sortField(line, separator string, field int) string {
	var fields []string
	if separator == "" {
		fields = strings.Fields(line)
	} else {
		fields = strings.Split(line, separator)
	}
	if field > len(fields) {
		return ""
	}
	return fields[field-1]
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
