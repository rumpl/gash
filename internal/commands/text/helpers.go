package text

import (
	"bufio"
	"fmt"
	"strings"

	"github.com/rumpl/gash/internal/command"
	"github.com/rumpl/gash/internal/commandutil"
)

type CommandContext = command.Context

func abs(ctx *CommandContext, name string) string {
	return commandutil.Abs(ctx, name)
}

func report(ctx *CommandContext, name string, err error) int {
	return commandutil.Report(ctx, name, err)
}

func readInputs(args []string, ctx *CommandContext) ([]byte, error) {
	return commandutil.ReadInputs(args, ctx)
}

func ioReadAll(ctx *CommandContext) (string, error) {
	var out strings.Builder
	_, err := bufio.NewReader(ctx.Stdin).WriteTo(&out)
	return out.String(), err
}

func missingOperand(ctx *CommandContext, name string) int {
	return report(ctx, name, fmt.Errorf("missing operand"))
}
