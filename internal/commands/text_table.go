package commands

import (
	"bufio"
	"context"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

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

func commandNL(_ context.Context, args []string, c *CommandContext) int {
	width := 6
	separator := "\t"
	numberBlank := false
	var files []string
	for i := 0; i < len(args); i++ {
		if args[i] == "-ba" {
			numberBlank = true
		} else if args[i] == "-w" && i+1 < len(args) {
			i++
			width, _ = strconv.Atoi(args[i])
		} else if args[i] == "-s" && i+1 < len(args) {
			i++
			separator = args[i]
		} else {
			files = append(files, args[i])
		}
	}
	data, err := readInputs(files, c)
	if err != nil {
		return report(c, "nl", err)
	}
	n := 1
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := scanner.Text()
		if line != "" || numberBlank {
			fmt.Fprintf(c.Stdout, "%*d%s%s\n", width, n, separator, line)
			n++
		} else {
			fmt.Fprintln(c.Stdout, line)
		}
	}
	return 0
}

func commandFold(_ context.Context, args []string, c *CommandContext) int {
	width := 80
	spaces := false
	var files []string
	for i := 0; i < len(args); i++ {
		if args[i] == "-w" && i+1 < len(args) {
			i++
			width, _ = strconv.Atoi(args[i])
		} else if strings.HasPrefix(args[i], "-w") && len(args[i]) > 2 {
			width, _ = strconv.Atoi(args[i][2:])
		} else if args[i] == "-s" {
			spaces = true
		} else {
			files = append(files, args[i])
		}
	}
	if width <= 0 {
		return report(c, "fold", fmt.Errorf("invalid width"))
	}
	data, err := readInputs(files, c)
	if err != nil {
		return report(c, "fold", err)
	}
	for _, line := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
		r := []rune(line)
		for len(r) > width {
			cut := width
			if spaces {
				for i := width; i > 0; i-- {
					if r[i-1] == ' ' || r[i-1] == '\t' {
						cut = i
						break
					}
				}
			}
			fmt.Fprintln(c.Stdout, string(r[:cut]))
			r = r[cut:]
			for spaces && len(r) > 0 && r[0] == ' ' {
				r = r[1:]
			}
		}
		fmt.Fprintln(c.Stdout, string(r))
	}
	return 0
}

func commandExpand(_ context.Context, args []string, c *CommandContext) int {
	return expandTabs(args, c, false)
}

func commandUnexpand(_ context.Context, args []string, c *CommandContext) int {
	return expandTabs(args, c, true)
}

func expandTabs(args []string, c *CommandContext, reverse bool) int {
	width := 8
	all := false
	var files []string
	for i := 0; i < len(args); i++ {
		if args[i] == "-a" {
			all = true
		} else if args[i] == "-t" && i+1 < len(args) {
			i++
			width, _ = strconv.Atoi(strings.Split(args[i], ",")[0])
		} else {
			files = append(files, args[i])
		}
	}
	data, err := readInputs(files, c)
	if err != nil {
		return report(c, "expand", err)
	}
	if !reverse {
		column := 0
		for _, r := range string(data) {
			if r == '\n' {
				fmt.Fprint(c.Stdout, "\n")
				column = 0
			} else if r == '\t' {
				n := width - column%width
				fmt.Fprint(c.Stdout, strings.Repeat(" ", n))
				column += n
			} else {
				fmt.Fprint(c.Stdout, string(r))
				column++
			}
		}
		return 0
	}
	for _, line := range strings.SplitAfter(string(data), "\n") {
		body := strings.TrimSuffix(line, "\n")
		limit := len(body)
		if !all {
			limit = 0
			for limit < len(body) && body[limit] == ' ' {
				limit++
			}
		}
		prefix := body[:limit]
		for strings.Contains(prefix, strings.Repeat(" ", width)) {
			prefix = strings.Replace(prefix, strings.Repeat(" ", width), "\t", 1)
		}
		fmt.Fprint(c.Stdout, prefix+body[limit:])
		if strings.HasSuffix(line, "\n") {
			fmt.Fprintln(c.Stdout)
		}
	}
	return 0
}

func commandColumn(_ context.Context, args []string, c *CommandContext) int {
	separator := ""
	table := false
	var files []string
	for i := 0; i < len(args); i++ {
		if args[i] == "-t" {
			table = true
		} else if args[i] == "-s" && i+1 < len(args) {
			i++
			separator = args[i]
		} else {
			files = append(files, args[i])
		}
	}
	data, err := readInputs(files, c)
	if err != nil {
		return report(c, "column", err)
	}
	if !table {
		fmt.Fprint(c.Stdout, string(data))
		return 0
	}
	var rows [][]string
	var widths []int
	for _, line := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
		fields := strings.Fields(line)
		if separator != "" {
			fields = strings.Split(line, separator)
		}
		rows = append(rows, fields)
		for i, v := range fields {
			if i >= len(widths) {
				widths = append(widths, 0)
			}
			if utf8.RuneCountInString(v) > widths[i] {
				widths[i] = utf8.RuneCountInString(v)
			}
		}
	}
	for _, row := range rows {
		for i, v := range row {
			if i == len(row)-1 {
				fmt.Fprint(c.Stdout, v)
			} else {
				fmt.Fprintf(c.Stdout, "%-*s  ", widths[i], v)
			}
		}
		fmt.Fprintln(c.Stdout)
	}
	return 0
}

func commandOD(_ context.Context, args []string, c *CommandContext) int {
	format := "o"
	var files []string
	for _, arg := range args {
		if arg == "-x" {
			format = "x"
		} else if arg == "-c" {
			format = "c"
		} else if strings.HasPrefix(arg, "-") {
			continue
		} else {
			files = append(files, arg)
		}
	}
	data, err := readInputs(files, c)
	if err != nil {
		return report(c, "od", err)
	}
	for offset := 0; offset < len(data); offset += 16 {
		end := offset + 16
		if end > len(data) {
			end = len(data)
		}
		fmt.Fprintf(c.Stdout, "%07o", offset)
		for _, b := range data[offset:end] {
			switch format {
			case "x":
				fmt.Fprintf(c.Stdout, " %02x", b)
			case "c":
				if b >= 32 && b < 127 {
					fmt.Fprintf(c.Stdout, "   %c", b)
				} else {
					fmt.Fprintf(c.Stdout, " %03o", b)
				}
			default:
				fmt.Fprintf(c.Stdout, " %03o", b)
			}
		}
		fmt.Fprintln(c.Stdout)
	}
	fmt.Fprintf(c.Stdout, "%07o\n", len(data))
	return 0
}
