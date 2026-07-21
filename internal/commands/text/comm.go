package text

import (
	"context"
	"fmt"
	"strings"

	gfs "github.com/rumpl/gash/pkg/fs"
)

func commandComm(_ context.Context, args []string, c *CommandContext) int {
	suppress := map[int]bool{}
	var files []string
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			for _, r := range arg[1:] {
				if r >= '1' && r <= '3' {
					suppress[int(r-'0')] = true
				}
			}
		} else {
			files = append(files, arg)
		}
	}
	if len(files) != 2 {
		return report(c, "comm", fmt.Errorf("expected two files"))
	}
	a, err := gfs.ReadFile(c.FS, abs(c, files[0]))
	if err != nil {
		return report(c, "comm", err)
	}
	b, err := gfs.ReadFile(c.FS, abs(c, files[1]))
	if err != nil {
		return report(c, "comm", err)
	}
	left, right := strings.Split(strings.TrimSuffix(string(a), "\n"), "\n"), strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
	i, j := 0, 0
	emit := func(column int, value string) {
		if suppress[column] {
			return
		}
		prefix := ""
		for col := 1; col < column; col++ {
			if !suppress[col] {
				prefix += "\t"
			}
		}
		fmt.Fprintln(c.Stdout, prefix+value)
	}
	for i < len(left) || j < len(right) {
		if i >= len(left) {
			emit(2, right[j])
			j++
		} else if j >= len(right) {
			emit(1, left[i])
			i++
		} else if left[i] == right[j] {
			emit(3, left[i])
			i++
			j++
		} else if left[i] < right[j] {
			emit(1, left[i])
			i++
		} else {
			emit(2, right[j])
			j++
		}
	}
	return 0
}
