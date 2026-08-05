// Package commandhelp formats the consistent help output used by built-in commands.
package commandhelp

import (
	"fmt"
	"strings"

	"github.com/rumpl/gash/internal/command"
)

type Info struct {
	Name        string
	Summary     string
	Usage       string
	Description []string
	Options     []string
	Examples    []string
	Notes       []string
}

func Requested(args []string) bool {
	for _, arg := range args {
		if arg == "--" {
			return false
		}
		if arg == "--help" {
			return true
		}
	}
	return false
}

func Show(ctx *command.Context, info Info) int {
	var output strings.Builder
	fmt.Fprintf(&output, "%s - %s\n\n", info.Name, info.Summary)
	fmt.Fprintf(&output, "Usage: %s\n", info.Usage)
	writeSection(&output, "Description", info.Description)
	writeSection(&output, "Options", info.Options)
	writeSection(&output, "Examples", info.Examples)
	writeSection(&output, "Notes", info.Notes)
	fmt.Fprint(ctx.Stdout, output.String())
	return 0
}

func UnknownOption(ctx *command.Context, name, option string) int {
	if strings.HasPrefix(option, "--") {
		fmt.Fprintf(ctx.Stderr, "%s: unrecognized option '%s'\n", name, option)
	} else {
		fmt.Fprintf(ctx.Stderr, "%s: invalid option -- '%s'\n", name, strings.TrimPrefix(option, "-"))
	}
	return 1
}

func writeSection(output *strings.Builder, name string, lines []string) {
	if len(lines) == 0 {
		return
	}
	fmt.Fprintf(output, "\n%s:\n", name)
	for _, line := range lines {
		if line == "" {
			output.WriteByte('\n')
			continue
		}
		fmt.Fprintf(output, "  %s\n", line)
	}
}
