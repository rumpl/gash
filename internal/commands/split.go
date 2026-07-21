package commands

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	gfs "github.com/rumpl/gash/pkg/fs"
)

func commandSplit(_ context.Context, args []string, c *CommandContext) int {
	mode := "lines"
	amount := 1000
	numeric := false
	suffixLen := 2
	additional := ""
	var positional []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-d":
			numeric = true
		case arg == "-l" || arg == "-b" || arg == "-n" || arg == "-a":
			if i+1 >= len(args) {
				return report(c, "split", fmt.Errorf("option requires an argument"))
			}
			i++
			value := args[i]
			if arg == "-a" {
				suffixLen, _ = strconv.Atoi(value)
			} else {
				mode = map[string]string{"-l": "lines", "-b": "bytes", "-n": "chunks"}[arg]
				amount = parseSize(value)
			}
		case strings.HasPrefix(arg, "--additional-suffix="):
			additional = strings.TrimPrefix(arg, "--additional-suffix=")
		default:
			positional = append(positional, arg)
		}
	}
	if amount <= 0 || suffixLen <= 0 {
		return report(c, "split", fmt.Errorf("invalid number"))
	}
	input, prefix := "-", "x"
	if len(positional) > 0 {
		input = positional[0]
	}
	if len(positional) > 1 {
		prefix = positional[1]
	}
	var data []byte
	var err error
	if input == "-" {
		data, err = io.ReadAll(c.Stdin)
	} else {
		data, err = gfs.ReadFile(c.FS, abs(c, input))
	}
	if err != nil {
		return report(c, "split", err)
	}
	var chunks [][]byte
	switch mode {
	case "bytes":
		for len(data) > 0 {
			n := amount
			if n > len(data) {
				n = len(data)
			}
			chunks = append(chunks, append([]byte(nil), data[:n]...))
			data = data[n:]
		}
	case "chunks":
		size := (len(data) + amount - 1) / amount
		if size > 0 {
			for len(data) > 0 {
				n := size
				if n > len(data) {
					n = len(data)
				}
				chunks = append(chunks, append([]byte(nil), data[:n]...))
				data = data[n:]
			}
		}
	default:
		start, lines := 0, 0
		for i, b := range data {
			if b == '\n' {
				lines++
				if lines == amount {
					chunks = append(chunks, append([]byte(nil), data[start:i+1]...))
					start = i + 1
					lines = 0
				}
			}
		}
		if start < len(data) {
			chunks = append(chunks, append([]byte(nil), data[start:]...))
		}
	}
	for i, chunk := range chunks {
		suffix := splitSuffix(i, numeric, suffixLen)
		if err := gfs.WriteFile(c.FS, abs(c, prefix+suffix+additional), chunk, 0o644); err != nil {
			return report(c, "split", err)
		}
	}
	return 0
}

func parseSize(value string) int {
	mult := 1
	if len(value) > 0 {
		switch value[len(value)-1] {
		case 'K', 'k':
			mult = 1024
			value = value[:len(value)-1]
		case 'M', 'm':
			mult = 1024 * 1024
			value = value[:len(value)-1]
		case 'G', 'g':
			mult = 1024 * 1024 * 1024
			value = value[:len(value)-1]
		}
	}
	n, _ := strconv.Atoi(value)
	return n * mult
}

func splitSuffix(index int, numeric bool, length int) string {
	if numeric {
		return fmt.Sprintf("%0*d", length, index)
	}
	out := make([]byte, length)
	for i := length - 1; i >= 0; i-- {
		out[i] = byte('a' + index%26)
		index /= 26
	}
	return string(out)
}
