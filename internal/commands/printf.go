package commands

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

func commandPrintf(ctx context.Context, args []string, c *CommandContext) int {
	select {
	case <-ctx.Done():
		return 124
	default:
	}
	if len(args) == 0 {
		fmt.Fprintln(c.Stderr, "printf: usage: printf format [arguments]")
		return 2
	}
	if args[0] == "--" {
		args = args[1:]
		if len(args) == 0 {
			fmt.Fprintln(c.Stderr, "printf: usage: printf format [arguments]")
			return 2
		}
	}
	format, stopped := decodePrintfEscapes(args[0])
	if stopped {
		return 0
	}
	values := args[1:]
	position := 0
	for {
		output, consumed, err := formatPrintfOnce(format, values, position)
		if err != nil {
			fmt.Fprintln(c.Stderr, "printf:", err)
			return 1
		}
		fmt.Fprint(c.Stdout, output)
		position += consumed
		if consumed == 0 || position >= len(values) {
			break
		}
	}
	return 0
}

func formatPrintfOnce(format string, values []string, start int) (string, int, error) {
	var output strings.Builder
	consumed := 0
	for index := 0; index < len(format); {
		if format[index] != '%' {
			output.WriteByte(format[index])
			index++
			continue
		}
		if index+1 < len(format) && format[index+1] == '%' {
			output.WriteByte('%')
			index += 2
			continue
		}
		end := index + 1
		for end < len(format) && strings.ContainsRune("-+ 0#'", rune(format[end])) {
			end++
		}
		for end < len(format) && format[end] >= '0' && format[end] <= '9' {
			end++
		}
		if end < len(format) && format[end] == '.' {
			end++
			for end < len(format) && format[end] >= '0' && format[end] <= '9' {
				end++
			}
		}
		if end >= len(format) {
			return "", consumed, fmt.Errorf("missing format character")
		}
		conversion := format[end]
		specification := format[index : end+1]
		value := ""
		if start+consumed < len(values) {
			value = values[start+consumed]
		}
		consumed++
		formatted, err := formatPrintfValue(specification, conversion, value)
		if err != nil {
			return "", consumed, err
		}
		output.WriteString(formatted)
		index = end + 1
	}
	return output.String(), consumed, nil
}

func formatPrintfValue(specification string, conversion byte, value string) (string, error) {
	switch conversion {
	case 's':
		return fmt.Sprintf(normalizeStringSpecification(specification), value), nil
	case 'c':
		if value == "" {
			return "", nil
		}
		r, _ := utf8.DecodeRuneInString(value)
		return fmt.Sprintf(specification, r), nil
	case 'd', 'i', 'u', 'o', 'x', 'X':
		number, err := parsePrintfInteger(value)
		if err != nil {
			return "", fmt.Errorf("%s: invalid number", value)
		}
		if conversion == 'i' || conversion == 'u' {
			specification = specification[:len(specification)-1] + "d"
		}
		return fmt.Sprintf(specification, number), nil
	case 'e', 'E', 'f', 'F', 'g', 'G':
		number := float64(0)
		if value != "" {
			parsed, err := strconv.ParseFloat(value, 64)
			if err != nil {
				return "", fmt.Errorf("%s: invalid number", value)
			}
			number = parsed
		}
		return fmt.Sprintf(specification, number), nil
	case 'b':
		decoded, _ := decodePrintfEscapes(value)
		return decoded, nil
	case 'q':
		quoted := shellQuote(value)
		return fmt.Sprintf(specification[:len(specification)-1]+"s", quoted), nil
	default:
		return "", fmt.Errorf("invalid format character %%%c", conversion)
	}
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	safe := true
	printable := true
	for _, character := range value {
		if !strings.ContainsRune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_@%+=:,./-", character) {
			safe = false
		}
		if character < 0x20 || character == 0x7f {
			printable = false
		}
	}
	if safe {
		return value
	}
	if printable {
		var quoted strings.Builder
		for _, character := range value {
			if !strings.ContainsRune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_@%+=:,./-", character) {
				quoted.WriteByte('\\')
			}
			quoted.WriteRune(character)
		}
		return quoted.String()
	}
	escaped := strconv.Quote(value)
	escaped = strings.TrimSuffix(strings.TrimPrefix(escaped, "\""), "\"")
	escaped = strings.ReplaceAll(escaped, "'", "\\'")
	return "$'" + escaped + "'"
}

func normalizeStringSpecification(specification string) string {
	if strings.HasPrefix(specification, "%0") {
		return "%" + strings.TrimPrefix(specification, "%0")
	}
	return specification
}

func parsePrintfInteger(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	if value[0] == '\'' || value[0] == '"' {
		if len(value) == 1 {
			return 0, nil
		}
		r, _ := utf8.DecodeRuneInString(value[1:])
		return int64(r), nil
	}
	base := 10
	digits := value
	negative := false
	if strings.HasPrefix(digits, "-") {
		negative = true
		digits = strings.TrimPrefix(digits, "-")
	} else {
		digits = strings.TrimPrefix(digits, "+")
	}
	if strings.HasPrefix(digits, "0x") || strings.HasPrefix(digits, "0X") {
		base = 16
		digits = digits[2:]
	} else if len(digits) > 1 && strings.HasPrefix(digits, "0") {
		base = 8
		digits = digits[1:]
	}
	number, err := strconv.ParseInt(digits, base, 64)
	if negative {
		number = -number
	}
	return number, err
}

func decodePrintfEscapes(value string) (string, bool) {
	var output strings.Builder
	for index := 0; index < len(value); index++ {
		if value[index] != '\\' || index+1 >= len(value) {
			output.WriteByte(value[index])
			continue
		}
		index++
		switch value[index] {
		case 'a':
			output.WriteByte('\a')
		case 'b':
			output.WriteByte('\b')
		case 'c':
			return output.String(), true
		case 'e', 'E':
			output.WriteByte(0x1b)
		case 'f':
			output.WriteByte('\f')
		case 'n':
			output.WriteByte('\n')
		case 'r':
			output.WriteByte('\r')
		case 't':
			output.WriteByte('\t')
		case 'v':
			output.WriteByte('\v')
		case '\\':
			output.WriteByte('\\')
		case 'x':
			start := index + 1
			end := start
			for end < len(value) && end < start+2 && isPrintfHexDigit(value[end]) {
				end++
			}
			if end == start {
				output.WriteString(`\x`)
				continue
			}
			number, _ := strconv.ParseUint(value[start:end], 16, 8)
			output.WriteByte(byte(number))
			index = end - 1
		case 'u', 'U':
			maximumDigits := 4
			if value[index] == 'U' {
				maximumDigits = 8
			}
			start := index + 1
			end := start
			for end < len(value) && end < start+maximumDigits && isPrintfHexDigit(value[end]) {
				end++
			}
			if end == start {
				output.WriteByte('\\')
				output.WriteByte(value[index])
				continue
			}
			number, _ := strconv.ParseUint(value[start:end], 16, 32)
			character := rune(number)
			if !utf8.ValidRune(character) {
				output.WriteString(value[index-1 : end])
			} else {
				output.WriteRune(character)
			}
			index = end - 1
		case '0':
			start := index + 1
			end := start
			for end < len(value) && end < start+3 && value[end] >= '0' && value[end] <= '7' {
				end++
			}
			if end == start {
				output.WriteByte(0)
				continue
			}
			number, _ := strconv.ParseUint(value[start:end], 8, 8)
			output.WriteByte(byte(number))
			index = end - 1
		default:
			output.WriteByte('\\')
			output.WriteByte(value[index])
		}
	}
	return output.String(), false
}

func isPrintfHexDigit(value byte) bool {
	return value >= '0' && value <= '9' || value >= 'a' && value <= 'f' || value >= 'A' && value <= 'F'
}
