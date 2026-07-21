package text

import (
	"context"
	"fmt"
	"strings"

	gfs "github.com/rumpl/gash/pkg/fs"
)

func commandPaste(_ context.Context, args []string, c *CommandContext) int {
	delimiter := "\t"
	serial := false
	var files []string
	for i := 0; i < len(args); i++ {
		if args[i] == "-d" && i+1 < len(args) {
			i++
			delimiter = args[i]
		} else if strings.HasPrefix(args[i], "-d") && len(args[i]) > 2 {
			delimiter = args[i][2:]
		} else if args[i] == "-s" {
			serial = true
		} else {
			files = append(files, args[i])
		}
	}
	if len(files) == 0 {
		files = []string{"-"}
	}
	columns := make([][]string, len(files))
	max := 0
	stdin, _ := ioReadAll(c)
	for i, file := range files {
		var data string
		if file == "-" {
			data = stdin
		} else {
			b, err := gfs.ReadFile(c.FS, abs(c, file))
			if err != nil {
				return report(c, "paste", err)
			}
			data = string(b)
		}
		columns[i] = strings.Split(strings.TrimSuffix(data, "\n"), "\n")
		if len(columns[i]) > max {
			max = len(columns[i])
		}
	}
	if serial {
		for _, col := range columns {
			fmt.Fprintln(c.Stdout, strings.Join(col, delimiter))
		}
		return 0
	}
	for row := 0; row < max; row++ {
		values := make([]string, len(columns))
		for i, col := range columns {
			if row < len(col) {
				values[i] = col[row]
			}
		}
		fmt.Fprintln(c.Stdout, strings.Join(values, delimiter))
	}
	return 0
}
