package gash

import (
	"math"
	"strconv"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

func normalizeArithmeticBases(program syntax.Node) {
	syntax.Walk(program, func(node syntax.Node) bool {
		switch arithmetic := node.(type) {
		case *syntax.ArithmExp:
			normalizeArithmeticExpression(arithmetic.X)
		case *syntax.ArithmCmd:
			normalizeArithmeticExpression(arithmetic.X)
		}
		return true
	})
}

func normalizeArithmeticExpression(expression syntax.ArithmExpr) {
	syntax.Walk(expression, func(node syntax.Node) bool {
		word, ok := node.(*syntax.Word)
		if !ok {
			return true
		}
		normalizeArithmeticWord(word)
		return true
	})
}

func normalizeArithmeticWord(word *syntax.Word) {
	if len(word.Parts) == 1 {
		if literal, ok := word.Parts[0].(*syntax.Lit); ok {
			if decimal, ok := parseStaticBaseNumber(literal.Value); ok {
				literal.Value = decimal
				return
			}
		}
	}

	// mvdan already interprets leading-zero values as shell arithmetic numbers.
	// Removing an explicit decimal prefix therefore preserves Bash's 10#$var
	// behavior for decimal variables such as 007 while avoiding mvdan's
	// unsupported '#' syntax.
	for index := 0; index+1 < len(word.Parts); index++ {
		literal, literalOK := word.Parts[index].(*syntax.Lit)
		_, parameterOK := word.Parts[index+1].(*syntax.ParamExp)
		if literalOK && parameterOK && strings.HasSuffix(literal.Value, "10#") {
			literal.Value = strings.TrimSuffix(literal.Value, "10#")
		}
	}
}

func parseStaticBaseNumber(value string) (string, bool) {
	separator := strings.IndexByte(value, '#')
	if separator <= 0 || separator == len(value)-1 {
		return "", false
	}
	base, err := strconv.Atoi(value[:separator])
	if err != nil || base < 2 || base > 64 {
		return "", false
	}
	number, ok := parseBashBaseDigits(value[separator+1:], base)
	if !ok {
		return "", false
	}
	return strconv.FormatInt(number, 10), true
}

func parseBashBaseDigits(digits string, base int) (int64, bool) {
	var number int64
	for _, digit := range digits {
		value, ok := bashDigitValue(digit, base)
		if !ok || value >= base || number > (math.MaxInt64-int64(value))/int64(base) {
			return 0, false
		}
		number = number*int64(base) + int64(value)
	}
	return number, true
}

func bashDigitValue(digit rune, base int) (int, bool) {
	switch {
	case digit >= '0' && digit <= '9':
		return int(digit - '0'), true
	case digit >= 'a' && digit <= 'z':
		return int(digit-'a') + 10, true
	case digit >= 'A' && digit <= 'Z' && base <= 36:
		return int(digit-'A') + 10, true
	case digit >= 'A' && digit <= 'Z':
		return int(digit-'A') + 36, true
	case digit == '@':
		return 62, true
	case digit == '_':
		return 63, true
	default:
		return 0, false
	}
}
