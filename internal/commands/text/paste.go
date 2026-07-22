package text

import (
	"context"
	"fmt"
	"strings"

	gfs "github.com/rumpl/gash/pkg/fs"
)

type pasteSource struct {
	lines []string
	next  int
}

func (s *pasteSource) readLine() (string, bool) {
	if s.next >= len(s.lines) {
		return "", false
	}
	line := s.lines[s.next]
	s.next++
	return line, true
}

func pasteLines(data string) []string {
	if data == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(data, "\n"), "\n")
}

func commandPaste(_ context.Context, args []string, c *CommandContext) int {
	delimiter := "\t"
	serial := false
	var files []string
	options := true
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !options || arg == "-" {
			files = append(files, arg)
			continue
		}
		if arg == "--" {
			options = false
			continue
		}
		if arg == "--serial" {
			serial = true
			continue
		}
		if arg == "--delimiters" {
			if i+1 >= len(args) {
				return report(c, "paste", fmt.Errorf("option requires an argument -- 'd'"))
			}
			i++
			delimiter = args[i]
			continue
		}
		if strings.HasPrefix(arg, "--delimiters=") {
			delimiter = strings.TrimPrefix(arg, "--delimiters=")
			continue
		}
		if strings.HasPrefix(arg, "-") && len(arg) > 1 {
			short := arg[1:]
			for j := 0; j < len(short); j++ {
				switch short[j] {
				case 's':
					serial = true
				case 'd':
					if j+1 < len(short) {
						delimiter = short[j+1:]
						j = len(short)
					} else {
						if i+1 >= len(args) {
							return report(c, "paste", fmt.Errorf("option requires an argument -- 'd'"))
						}
						i++
						delimiter = args[i]
					}
				default:
					return report(c, "paste", fmt.Errorf("invalid option -- '%c'", short[j]))
				}
			}
			continue
		}
		files = append(files, arg)
	}
	if len(files) == 0 {
		files = []string{"-"}
	}

	sources := make([]*pasteSource, 0, len(files))
	var stdin *pasteSource
	for _, file := range files {
		if file == "-" {
			// All '-' operands refer to the same stream. In parallel mode each
			// occurrence reads the next record rather than replaying stdin.
			if stdin == nil {
				data, err := ioReadAll(c)
				if err != nil {
					return report(c, "paste", err)
				}
				stdin = &pasteSource{lines: pasteLines(data)}
			}
			sources = append(sources, stdin)
			continue
		}
		b, err := gfs.ReadFile(c.FS, abs(c, file))
		if err != nil {
			return report(c, "paste", err)
		}
		sources = append(sources, &pasteSource{lines: pasteLines(string(b))})
	}

	if serial {
		for _, source := range sources {
			var lines []string
			for {
				line, ok := source.readLine()
				if !ok {
					break
				}
				lines = append(lines, line)
			}
			if len(lines) > 0 {
				fmt.Fprintln(c.Stdout, strings.Join(lines, delimiter))
			}
		}
		return 0
	}

	for {
		values := make([]string, len(sources))
		readAny := false
		for i, source := range sources {
			if line, ok := source.readLine(); ok {
				values[i] = line
				readAny = true
			}
		}
		if !readAny {
			break
		}
		fmt.Fprintln(c.Stdout, strings.Join(values, delimiter))
	}
	return 0
}
