package commands

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf16"
)

func commandExpr(_ context.Context, args []string, c *CommandContext) int {
	if len(args) == 0 {
		fmt.Fprint(c.Stderr, "expr: missing operand\n")
		return 2
	}

	result, err := evaluateExpr(args)
	if err != nil {
		fmt.Fprintf(c.Stderr, "expr: %s\n", sanitizeExprError(err.Error()))
		return 2
	}

	fmt.Fprintln(c.Stdout, result)
	if result == "0" || result == "" {
		return 1
	}
	return 0
}

type exprParser struct {
	args []string
	i    int
}

func evaluateExpr(args []string) (string, error) {
	if len(args) == 1 {
		return args[0], nil
	}

	p := &exprParser{args: args}
	result, err := p.parseOr()
	if err != nil {
		return "", err
	}
	if p.i != len(args) {
		return "", fmt.Errorf("syntax error")
	}
	return result, nil
}

func (p *exprParser) parseOr() (string, error) {
	left, err := p.parseAnd()
	if err != nil {
		return "", err
	}
	for p.i < len(p.args) && p.args[p.i] == "|" {
		p.i++
		right, err := p.parseAnd()
		if err != nil {
			return "", err
		}
		if left != "0" && left != "" {
			return left, nil
		}
		left = right
	}
	return left, nil
}

func (p *exprParser) parseAnd() (string, error) {
	left, err := p.parseComparison()
	if err != nil {
		return "", err
	}
	for p.i < len(p.args) && p.args[p.i] == "&" {
		p.i++
		right, err := p.parseComparison()
		if err != nil {
			return "", err
		}
		if left == "0" || left == "" || right == "0" || right == "" {
			left = "0"
		}
	}
	return left, nil
}

func (p *exprParser) parseComparison() (string, error) {
	left, err := p.parseAddSub()
	if err != nil {
		return "", err
	}
	for p.i < len(p.args) {
		op := p.args[p.i]
		if op != "=" && op != "!=" && op != "<" && op != ">" && op != "<=" && op != ">=" {
			break
		}
		p.i++
		right, err := p.parseAddSub()
		if err != nil {
			return "", err
		}

		leftNum, leftOK := parseExprInt(left)
		rightNum, rightOK := parseExprInt(right)
		isNumeric := leftOK && rightOK

		var result bool
		switch op {
		case "=":
			if isNumeric {
				result = leftNum == rightNum
			} else {
				result = left == right
			}
		case "!=":
			if isNumeric {
				result = leftNum != rightNum
			} else {
				result = left != right
			}
		case "<":
			if isNumeric {
				result = leftNum < rightNum
			} else {
				result = left < right
			}
		case ">":
			if isNumeric {
				result = leftNum > rightNum
			} else {
				result = left > right
			}
		case "<=":
			if isNumeric {
				result = leftNum <= rightNum
			} else {
				result = left <= right
			}
		case ">=":
			if isNumeric {
				result = leftNum >= rightNum
			} else {
				result = left >= right
			}
		}
		if result {
			left = "1"
		} else {
			left = "0"
		}
	}
	return left, nil
}

func (p *exprParser) parseAddSub() (string, error) {
	left, err := p.parseMulDiv()
	if err != nil {
		return "", err
	}
	for p.i < len(p.args) {
		op := p.args[p.i]
		if op != "+" && op != "-" {
			break
		}
		p.i++
		right, err := p.parseMulDiv()
		if err != nil {
			return "", err
		}
		leftNum, leftOK := parseExprInt(left)
		rightNum, rightOK := parseExprInt(right)
		if !leftOK || !rightOK {
			return "", fmt.Errorf("non-integer argument")
		}
		if op == "+" {
			left = strconv.FormatInt(leftNum+rightNum, 10)
		} else {
			left = strconv.FormatInt(leftNum-rightNum, 10)
		}
	}
	return left, nil
}

func (p *exprParser) parseMulDiv() (string, error) {
	left, err := p.parseMatch()
	if err != nil {
		return "", err
	}
	for p.i < len(p.args) {
		op := p.args[p.i]
		if op != "*" && op != "/" && op != "%" {
			break
		}
		p.i++
		right, err := p.parseMatch()
		if err != nil {
			return "", err
		}
		leftNum, leftOK := parseExprInt(left)
		rightNum, rightOK := parseExprInt(right)
		if !leftOK || !rightOK {
			return "", fmt.Errorf("non-integer argument")
		}
		if (op == "/" || op == "%") && rightNum == 0 {
			return "", fmt.Errorf("division by zero")
		}
		switch op {
		case "*":
			left = strconv.FormatInt(leftNum*rightNum, 10)
		case "/":
			left = strconv.FormatInt(leftNum/rightNum, 10)
		case "%":
			left = strconv.FormatInt(leftNum%rightNum, 10)
		}
	}
	return left, nil
}

func (p *exprParser) parseMatch() (string, error) {
	left, err := p.parsePrimary()
	if err != nil {
		return "", err
	}
	for p.i < len(p.args) && p.args[p.i] == ":" {
		p.i++
		pattern, err := p.parsePrimary()
		if err != nil {
			return "", err
		}
		left, err = exprRegexMatch(left, "^"+pattern)
		if err != nil {
			return "", err
		}
	}
	return left, nil
}

func (p *exprParser) parsePrimary() (string, error) {
	if p.i >= len(p.args) {
		return "", fmt.Errorf("syntax error")
	}

	token := p.args[p.i]
	switch token {
	case "match":
		p.i++
		str, err := p.parsePrimary()
		if err != nil {
			return "", err
		}
		pattern, err := p.parsePrimary()
		if err != nil {
			return "", err
		}
		return exprRegexMatch(str, pattern)
	case "substr":
		p.i++
		str, err := p.parsePrimary()
		if err != nil {
			return "", err
		}
		posText, err := p.parsePrimary()
		if err != nil {
			return "", err
		}
		lenText, err := p.parsePrimary()
		if err != nil {
			return "", err
		}
		pos, ok := parseExprInt(posText)
		if !ok {
			return "", fmt.Errorf("non-integer argument")
		}
		length, ok := parseExprInt(lenText)
		if !ok {
			return "", fmt.Errorf("non-integer argument")
		}
		return exprSubstring(str, pos, length), nil
	case "index":
		p.i++
		str, err := p.parsePrimary()
		if err != nil {
			return "", err
		}
		chars, err := p.parsePrimary()
		if err != nil {
			return "", err
		}
		return exprIndex(str, chars), nil
	case "length":
		p.i++
		str, err := p.parsePrimary()
		if err != nil {
			return "", err
		}
		return strconv.Itoa(exprUTF16Len(str)), nil
	case "(":
		p.i++
		result, err := p.parseOr()
		if err != nil {
			return "", err
		}
		if p.i >= len(p.args) || p.args[p.i] != ")" {
			return "", fmt.Errorf("syntax error")
		}
		p.i++
		return result, nil
	default:
		p.i++
		return token, nil
	}
}

func exprRegexMatch(str, pattern string) (string, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return "", err
	}
	match := re.FindStringSubmatch(str)
	if match == nil {
		return "0", nil
	}
	if len(match) > 1 {
		return match[1], nil
	}
	return strconv.Itoa(exprUTF16Len(match[0])), nil
}

func exprSubstring(str string, pos, length int64) string {
	units := utf16.Encode([]rune(str))
	start := pos - 1
	end := start + length
	if start < 0 {
		start = 0
	}
	if end < start {
		end = start
	}
	unitLen := int64(len(units))
	if start > unitLen {
		start = unitLen
	}
	if end > unitLen {
		end = unitLen
	}
	return string(utf16.Decode(units[start:end]))
}

func exprIndex(str, chars string) string {
	set := make(map[uint16]struct{}, exprUTF16Len(chars))
	for _, unit := range utf16.Encode([]rune(chars)) {
		set[unit] = struct{}{}
	}
	for i, unit := range utf16.Encode([]rune(str)) {
		if _, ok := set[unit]; ok {
			return strconv.Itoa(i + 1)
		}
	}
	return "0"
}

func exprUTF16Len(str string) int {
	return len(utf16.Encode([]rune(str)))
}

func parseExprInt(s string) (int64, bool) {
	s = strings.TrimLeftFunc(s, unicode.IsSpace)
	if s == "" {
		return 0, false
	}
	end := 0
	if s[0] == '+' || s[0] == '-' {
		end = 1
	}
	startDigits := end
	for end < len(s) && s[end] >= '0' && s[end] <= '9' {
		end++
	}
	if end == startDigits {
		return 0, false
	}
	n, err := strconv.ParseInt(s[:end], 10, 64)
	return n, err == nil
}

func sanitizeExprError(message string) string {
	if message == "" {
		return message
	}
	lines := strings.Split(message, "\n")
	return lines[0]
}
