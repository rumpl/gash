package text

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/rumpl/gash/internal/commandhelp"
	gfs "github.com/rumpl/gash/pkg/fs"
)

type diffOptions struct {
	brief      bool
	reportSame bool
	ignoreCase bool
	files      []string
}

type diffLine struct {
	text       string
	hasNewline bool
}

type diffOp struct {
	kind    byte
	oldLine *diffLine
	newLine *diffLine
}

func commandDiff(_ context.Context, args []string, c *CommandContext) int {
	if commandhelp.Requested(args) {
		if info, ok := commandhelp.Lookup("diff"); ok {
			return commandhelp.Show(c, info)
		}
	}

	opts, code := parseDiffArgs(args, c)
	if code != 0 {
		return code
	}
	if len(opts.files) != 2 {
		fmt.Fprint(c.Stderr, "diff: missing operand\n")
		return 2
	}

	f1, f2 := opts.files[0], opts.files[1]
	var stdin []byte
	var stdinRead bool
	readOperand := func(name string) (string, bool) {
		if name == "-" {
			if !stdinRead {
				data, err := io.ReadAll(c.Stdin)
				if err != nil {
					fmt.Fprintf(c.Stderr, "diff: %s: No such file or directory\n", name)
					return "", false
				}
				stdin = data
				stdinRead = true
			}
			return string(stdin), true
		}
		data, err := gfs.ReadFile(c.FS, abs(c, name))
		if err != nil {
			// just-bash reports any read failure here as a missing file, including
			// virtual directories or non-readable operands.
			fmt.Fprintf(c.Stderr, "diff: %s: No such file or directory\n", name)
			return "", false
		}
		return string(data), true
	}

	c1, ok := readOperand(f1)
	if !ok {
		return 2
	}
	c2, ok := readOperand(f2)
	if !ok {
		return 2
	}

	t1, t2 := c1, c2
	if opts.ignoreCase {
		t1 = strings.ToLower(t1)
		t2 = strings.ToLower(t2)
	}
	if t1 == t2 {
		if opts.reportSame {
			fmt.Fprintf(c.Stdout, "Files %s and %s are identical\n", f1, f2)
		}
		return 0
	}
	if opts.brief {
		fmt.Fprintf(c.Stdout, "Files %s and %s differ\n", f1, f2)
		return 1
	}

	fmt.Fprint(c.Stdout, createUnifiedDiff(f1, f2, c1, c2))
	return 1
}

func parseDiffArgs(args []string, c *CommandContext) (diffOptions, int) {
	var opts diffOptions
	for _, arg := range args {
		if strings.HasPrefix(arg, "--") {
			switch arg {
			case "--unified":
			case "--brief":
				opts.brief = true
			case "--report-identical-files":
				opts.reportSame = true
			case "--ignore-case":
				opts.ignoreCase = true
			default:
				return opts, commandhelp.UnknownOption(c, "diff", arg)
			}
			continue
		}
		if strings.HasPrefix(arg, "-") && arg != "-" {
			for _, r := range arg[1:] {
				switch r {
				case 'u':
				case 'q':
					opts.brief = true
				case 's':
					opts.reportSame = true
				case 'i':
					opts.ignoreCase = true
				default:
					return opts, commandhelp.UnknownOption(c, "diff", "-"+string(r))
				}
			}
			continue
		}
		opts.files = append(opts.files, arg)
	}
	return opts, 0
}

func createUnifiedDiff(oldName, newName, oldText, newText string) string {
	oldLines := splitDiffLines(oldText)
	newLines := splitDiffLines(newText)
	ops := diffLineOps(oldLines, newLines)

	var out strings.Builder
	out.WriteString("===================================================================\n")
	fmt.Fprintf(&out, "--- %s\n", oldName)
	fmt.Fprintf(&out, "+++ %s\n", newName)
	oldStart := 1
	newStart := 1
	if len(oldLines) == 0 {
		oldStart = 0
	}
	if len(newLines) == 0 {
		newStart = 0
	}
	fmt.Fprintf(&out, "@@ -%d,%d +%d,%d @@\n", oldStart, len(oldLines), newStart, len(newLines))
	for _, op := range ops {
		switch op.kind {
		case ' ':
			writeDiffLine(&out, ' ', *op.oldLine)
		case '-':
			writeDiffLine(&out, '-', *op.oldLine)
		case '+':
			writeDiffLine(&out, '+', *op.newLine)
		}
	}
	return out.String()
}

func splitDiffLines(s string) []diffLine {
	if s == "" {
		return nil
	}
	parts := strings.SplitAfter(s, "\n")
	lines := make([]diffLine, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			continue
		}
		hasNewline := strings.HasSuffix(part, "\n")
		text := strings.TrimSuffix(part, "\n")
		lines = append(lines, diffLine{text: text, hasNewline: hasNewline})
	}
	return lines
}

func diffLineOps(oldLines, newLines []diffLine) []diffOp {
	m, n := len(oldLines), len(newLines)
	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
	}
	for i := m - 1; i >= 0; i-- {
		for j := n - 1; j >= 0; j-- {
			if oldLines[i] == newLines[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else if dp[i+1][j] >= dp[i][j+1] {
				dp[i][j] = dp[i+1][j]
			} else {
				dp[i][j] = dp[i][j+1]
			}
		}
	}
	ops := make([]diffOp, 0, m+n)
	for i, j := 0, 0; i < m || j < n; {
		switch {
		case i < m && j < n && oldLines[i] == newLines[j]:
			oldLine := oldLines[i]
			newLine := newLines[j]
			ops = append(ops, diffOp{kind: ' ', oldLine: &oldLine, newLine: &newLine})
			i++
			j++
		case j < n && (i == m || dp[i][j+1] > dp[i+1][j]):
			newLine := newLines[j]
			ops = append(ops, diffOp{kind: '+', newLine: &newLine})
			j++
		case i < m:
			oldLine := oldLines[i]
			ops = append(ops, diffOp{kind: '-', oldLine: &oldLine})
			i++
		}
	}
	return ops
}

func writeDiffLine(out *strings.Builder, prefix byte, line diffLine) {
	out.WriteByte(prefix)
	out.WriteString(line.text)
	out.WriteByte('\n')
	if !line.hasNewline {
		out.WriteString("\\ No newline at end of file\n")
	}
}

// Deferred just-bash diff differences: gash formats changed files as a unified
// patch generated by its Go LCS implementation instead of depending on the
// upstream JavaScript diff package, so hunk grouping may differ for large files.
