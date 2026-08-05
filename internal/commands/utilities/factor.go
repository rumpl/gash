package utilities

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strconv"

	"github.com/rumpl/gash/internal/command"
)

// maxFactorInput keeps trial division bounded to at most one million candidate
// divisors for prime inputs.
const maxFactorInput uint64 = 1_000_000_000_000

func FactorCommand() command.Command {
	return command.Command{Name: "factor", Run: commandFactor}
}

func commandFactor(ctx context.Context, args []string, commandCtx *command.Context) int {
	tokens := args
	if len(tokens) == 0 {
		var err error
		tokens, err = readFactorTokens(commandCtx.Stdin)
		if err != nil {
			fmt.Fprintf(commandCtx.Stderr, "factor: %v\n", err)
			return 1
		}
	}

	for _, token := range tokens {
		number, err := parseFactorInput(token)
		if err != nil {
			fmt.Fprintf(commandCtx.Stderr, "factor: %q: %v\n", token, err)
			return 1
		}
		factors, err := primeFactors(ctx, number)
		if err != nil {
			fmt.Fprintln(commandCtx.Stderr, "factor: canceled")
			return 1
		}
		fmt.Fprintf(commandCtx.Stdout, "%d:", number)
		for _, factor := range factors {
			fmt.Fprintf(commandCtx.Stdout, " %d", factor)
		}
		fmt.Fprintln(commandCtx.Stdout)
	}
	return 0
}

func readFactorTokens(reader io.Reader) ([]string, error) {
	scanner := bufio.NewScanner(reader)
	scanner.Split(bufio.ScanWords)
	scanner.Buffer(make([]byte, 1024), 64*1024)
	var tokens []string
	for scanner.Scan() {
		tokens = append(tokens, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("cannot read input: %w", err)
	}
	return tokens, nil
}

func parseFactorInput(token string) (uint64, error) {
	if token == "" {
		return 0, fmt.Errorf("not a non-negative decimal integer")
	}
	for _, char := range token {
		if char < '0' || char > '9' {
			return 0, fmt.Errorf("not a non-negative decimal integer")
		}
	}
	number, err := strconv.ParseUint(token, 10, 64)
	if err != nil || number > maxFactorInput {
		return 0, fmt.Errorf("value exceeds maximum %d", maxFactorInput)
	}
	return number, nil
}

func primeFactors(ctx context.Context, number uint64) ([]uint64, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	if number < 2 {
		return nil, nil
	}
	factors := make([]uint64, 0, 16)
	for number%2 == 0 {
		factors = append(factors, 2)
		number /= 2
	}
	for divisor := uint64(3); divisor <= number/divisor; divisor += 2 {
		if divisor%1024 == 1 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}
		}
		for number%divisor == 0 {
			factors = append(factors, divisor)
			number /= divisor
		}
	}
	if number > 1 {
		factors = append(factors, number)
	}
	return factors, nil
}
