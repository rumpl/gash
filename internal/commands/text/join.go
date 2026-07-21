package text

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	gfs "github.com/rumpl/gash/pkg/fs"
)

func commandJoin(_ context.Context, args []string, c *CommandContext) int {
	separator := " "
	field1, field2 := 1, 1
	var files []string
	for i := 0; i < len(args); i++ {
		if strings.HasPrefix(args[i], "-t") && len(args[i]) > 2 {
			separator = args[i][2:]
			continue
		}
		switch args[i] {
		case "-t":
			if i+1 < len(args) {
				i++
				separator = args[i]
			}
		case "-1":
			if i+1 < len(args) {
				i++
				field1, _ = strconv.Atoi(args[i])
			}
		case "-2":
			if i+1 < len(args) {
				i++
				field2, _ = strconv.Atoi(args[i])
			}
		default:
			files = append(files, args[i])
		}
	}
	if len(files) != 2 {
		return report(c, "join", fmt.Errorf("missing file operand"))
	}
	read := func(name string) ([][]string, error) {
		data, err := gfs.ReadFile(c.FS, abs(c, name))
		if err != nil {
			return nil, err
		}
		var rows [][]string
		for _, line := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
			if separator == " " {
				rows = append(rows, strings.Fields(line))
			} else {
				rows = append(rows, strings.Split(line, separator))
			}
		}
		return rows, nil
	}
	a, err := read(files[0])
	if err != nil {
		return report(c, "join", err)
	}
	b, err := read(files[1])
	if err != nil {
		return report(c, "join", err)
	}
	index := map[string][][]string{}
	for _, row := range b {
		if field2 > 0 && field2 <= len(row) {
			index[row[field2-1]] = append(index[row[field2-1]], row)
		}
	}
	for _, left := range a {
		if field1 < 1 || field1 > len(left) {
			continue
		}
		key := left[field1-1]
		for _, right := range index[key] {
			out := []string{key}
			for i, v := range left {
				if i != field1-1 {
					out = append(out, v)
				}
			}
			for i, v := range right {
				if i != field2-1 {
					out = append(out, v)
				}
			}
			fmt.Fprintln(c.Stdout, strings.Join(out, separator))
		}
	}
	return 0
}
