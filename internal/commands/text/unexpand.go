package text

import "context"

func commandUnexpand(_ context.Context, args []string, ctx *CommandContext) int {
	return expandTabs(args, ctx, true)
}
