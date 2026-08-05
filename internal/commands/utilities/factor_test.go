package utilities

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/rumpl/gash/internal/command"
)

func TestFactorArguments(t *testing.T) {
	result := runFactorTest(context.Background(), []string{"12", "97"}, "")
	if result.code != 0 || result.stdout != "12: 2 2 3\n97: 97\n" || result.stderr != "" {
		t.Fatalf("result=%+v", result)
	}
}

func TestFactorReadsWhitespaceSeparatedStdin(t *testing.T) {
	result := runFactorTest(context.Background(), nil, "15  21\n")
	if result.code != 0 || result.stdout != "15: 3 5\n21: 3 7\n" || result.stderr != "" {
		t.Fatalf("result=%+v", result)
	}
}

func TestFactorZeroAndOneHaveNoFactors(t *testing.T) {
	result := runFactorTest(context.Background(), []string{"0", "1"}, "")
	if result.code != 0 || result.stdout != "0:\n1:\n" || result.stderr != "" {
		t.Fatalf("result=%+v", result)
	}
}

func TestFactorRejectsInvalidInputAndStops(t *testing.T) {
	for _, token := range []string{"-2", "1.5", "18446744073709551616", "1000000000001"} {
		t.Run(token, func(t *testing.T) {
			result := runFactorTest(context.Background(), []string{token}, "")
			if result.code == 0 || !strings.Contains(result.stderr, token) {
				t.Fatalf("result=%+v", result)
			}
		})
	}
}

func TestPrimeFactorsHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := primeFactors(ctx, 999999996989); err == nil {
		t.Fatal("expected cancellation error")
	}
}

type factorTestResult struct {
	stdout string
	stderr string
	code   int
}

func runFactorTest(ctx context.Context, args []string, stdin string) factorTestResult {
	var stdout, stderr bytes.Buffer
	commandCtx := &command.Context{
		Stdin:  strings.NewReader(stdin),
		Stdout: &stdout,
		Stderr: &stderr,
	}
	code := commandFactor(ctx, args, commandCtx)
	return factorTestResult{stdout: stdout.String(), stderr: stderr.String(), code: code}
}
