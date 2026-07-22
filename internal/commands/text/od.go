package text

import (
	"context"
	"fmt"
	"strings"

	"github.com/rumpl/gash/internal/commandhelp"
)

func commandOD(_ context.Context, args []string, c *CommandContext) int {
	format := "o1"
	addressFormat := "o"
	var files []string
	options := true
	for index := 0; index < len(args); index++ {
		argument := args[index]
		if options && argument == "--" {
			options = false
			continue
		}
		if !options || argument == "-" || !strings.HasPrefix(argument, "-") {
			files = append(files, argument)
			continue
		}
		switch {
		case argument == "-x":
			format = "x1"
		case argument == "-c":
			format = "c"
		case argument == "-A":
			if index+1 >= len(args) {
				fmt.Fprintln(c.Stderr, "od: option requires an argument -- 'A'")
				return 1
			}
			index++
			if !validODAddressFormat(args[index]) {
				return commandhelp.UnknownOption(c, "od", "-A"+args[index])
			}
			addressFormat = args[index]
		case strings.HasPrefix(argument, "-A") && len(argument) > 2:
			value := strings.TrimPrefix(argument, "-A")
			if !validODAddressFormat(value) {
				return commandhelp.UnknownOption(c, "od", argument)
			}
			addressFormat = value
		case argument == "-t":
			if index+1 >= len(args) {
				fmt.Fprintln(c.Stderr, "od: option requires an argument -- 't'")
				return 1
			}
			index++
			if !validODType(args[index]) {
				fmt.Fprintf(c.Stderr, "od: unsupported output type %q\n", args[index])
				return 1
			}
			format = args[index]
		case strings.HasPrefix(argument, "-t") && len(argument) > 2:
			value := strings.TrimPrefix(argument, "-t")
			if !validODType(value) {
				fmt.Fprintf(c.Stderr, "od: unsupported output type %q\n", value)
				return 1
			}
			format = value
		default:
			return commandhelp.UnknownOption(c, "od", argument)
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
		if addressFormat != "n" {
			fmt.Fprint(c.Stdout, formatODAddress(offset, addressFormat))
		}
		for _, value := range data[offset:end] {
			switch format {
			case "x1":
				fmt.Fprintf(c.Stdout, " %02x", value)
			case "c":
				if value >= 32 && value < 127 {
					fmt.Fprintf(c.Stdout, "   %c", value)
				} else {
					fmt.Fprintf(c.Stdout, " %03o", value)
				}
			default:
				fmt.Fprintf(c.Stdout, " %03o", value)
			}
		}
		fmt.Fprintln(c.Stdout)
	}
	if addressFormat != "n" {
		fmt.Fprintln(c.Stdout, formatODAddress(len(data), addressFormat))
	}
	return 0
}

func validODAddressFormat(value string) bool {
	return value == "d" || value == "n" || value == "o" || value == "x"
}

func validODType(value string) bool {
	return value == "c" || value == "o1" || value == "x1"
}

func formatODAddress(offset int, format string) string {
	switch format {
	case "d":
		return fmt.Sprintf("%07d", offset)
	case "x":
		return fmt.Sprintf("%07x", offset)
	default:
		return fmt.Sprintf("%07o", offset)
	}
}
