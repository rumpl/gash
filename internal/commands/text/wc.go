package text

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/rumpl/gash/internal/commandhelp"
)

func commandWC(_ context.Context, args []string, c *CommandContext) int {
	selected := map[rune]bool{}
	var files []string
	options := true
	for _, argument := range args {
		if options && argument == "--" {
			options = false
			continue
		}
		if options {
			var flag rune
			switch argument {
			case "--bytes":
				flag = 'c'
			case "--chars":
				flag = 'm'
			case "--lines":
				flag = 'l'
			case "--words":
				flag = 'w'
			}
			if flag != 0 {
				selected[flag] = true
				continue
			}
			if strings.HasPrefix(argument, "-") && argument != "-" {
				for _, short := range strings.TrimPrefix(argument, "-") {
					switch short {
					case 'c', 'm', 'l', 'w':
						selected[short] = true
					default:
						return commandhelp.UnknownOption(c, "wc", "-"+string(short))
					}
				}
				continue
			}
		}
		files = append(files, argument)
	}
	data, err := readInputs(files, c)
	if err != nil {
		return report(c, "wc", err)
	}
	if len(selected) == 0 {
		selected['l'] = true
		selected['w'] = true
		selected['c'] = true
	}
	counts := map[rune]int{
		'l': bytesCount(data, '\n'),
		'w': len(strings.Fields(string(data))),
		'm': utf8.RuneCount(data),
		'c': len(data),
	}
	first := true
	for _, flag := range []rune{'l', 'w', 'm', 'c'} {
		if !selected[flag] {
			continue
		}
		if !first {
			fmt.Fprint(c.Stdout, " ")
		}
		fmt.Fprint(c.Stdout, counts[flag])
		first = false
	}
	fmt.Fprintln(c.Stdout)
	return 0
}

func bytesCount(d []byte, b byte) int {
	n := 0
	for _, v := range d {
		if v == b {
			n++
		}
	}
	return n
}
