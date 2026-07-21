package text

import (
	"context"
	"fmt"
	"strings"
)

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
