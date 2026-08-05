package files

import (
	"context"
	"fmt"
	"strings"

	"github.com/rumpl/gash/internal/commandhelp"
	gfs "github.com/rumpl/gash/pkg/fs"
)

func commandUnlink(_ context.Context, args []string, c *CommandContext) int {
	if commandhelp.Requested(args) {
		info, _ := commandhelp.Lookup("unlink")
		return commandhelp.Show(c, info)
	}

	var operands []string
	optionsEnded := false
	for _, arg := range args {
		if !optionsEnded && arg == "--" {
			optionsEnded = true
			continue
		}
		if !optionsEnded && strings.HasPrefix(arg, "-") {
			return commandhelp.UnknownOption(c, "unlink", arg)
		}
		operands = append(operands, arg)
	}

	if len(operands) == 0 {
		fmt.Fprintln(c.Stderr, "unlink: missing operand")
		return 1
	}
	if len(operands) > 1 {
		fmt.Fprintf(c.Stderr, "unlink: extra operand '%s'\n", operands[1])
		return 1
	}

	name := operands[0]
	full := abs(c, name)
	info, err := gfs.Lstat(c.FS, full)
	if err != nil {
		return report(c, "unlink: "+name, err)
	}
	if info.IsDir() {
		fmt.Fprintf(c.Stderr, "unlink: cannot unlink '%s': Is a directory\n", name)
		return 1
	}
	if err := gfs.Remove(c.FS, full); err != nil {
		return report(c, "unlink: "+name, err)
	}
	return 0
}
