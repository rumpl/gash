package files

import (
	"context"
	"fmt"
	"path"
	"strconv"
	"strings"

	gfs "github.com/rumpl/gash/pkg/fs"
)

func commandDu(_ context.Context, args []string, c *CommandContext) int {
	all, human, summary, total := false, false, false, false
	var targets []string
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			all = all || strings.Contains(arg, "a")
			human = human || strings.Contains(arg, "h")
			summary = summary || strings.Contains(arg, "s")
			total = total || strings.Contains(arg, "c")
		} else {
			targets = append(targets, arg)
		}
	}
	if len(targets) == 0 {
		targets = []string{"."}
	}
	grand, code := int64(0), 0
	for _, target := range targets {
		size, lines, err := duWalk(c, abs(c, target), target, all, !summary, human)
		if err != nil {
			fmt.Fprintf(c.Stderr, "du: cannot access '%s': No such file or directory\n", target)
			code = 1
			continue
		}
		grand += size
		if summary {
			fmt.Fprintf(c.Stdout, "%s\t%s\n", formatSize(size, human), target)
		} else {
			for _, line := range lines {
				fmt.Fprintln(c.Stdout, line)
			}
		}
	}
	if total {
		fmt.Fprintf(c.Stdout, "%s\ttotal\n", formatSize(grand, human))
	}
	return code
}

func duWalk(c *CommandContext, name, display string, all, recurse, human bool) (int64, []string, error) {
	info, err := gfs.Stat(c.FS, name)
	if err != nil {
		return 0, nil, err
	}
	if !info.IsDir() {
		lines := []string{}
		if all {
			lines = append(lines, formatSize(info.Size(), human)+"\t"+display)
		}
		return info.Size(), lines, nil
	}
	entries, err := gfs.ReadDir(c.FS, name)
	if err != nil {
		return 0, nil, err
	}
	total := int64(0)
	var lines []string
	for _, e := range entries {
		s, sub, err := duWalk(c, path.Join(name, e.Name()), path.Join(display, e.Name()), all, recurse, human)
		if err != nil {
			return 0, nil, err
		}
		total += s
		lines = append(lines, sub...)
	}
	if recurse {
		lines = append(lines, formatSize(total, human)+"\t"+display)
	}
	return total, lines, nil
}

func formatSize(size int64, human bool) string {
	if !human {
		blocks := (size + 1023) / 1024
		if blocks == 0 {
			blocks = 1
		}
		return strconv.FormatInt(blocks, 10)
	}
	units := []string{"", "K", "M", "G"}
	v := float64(size)
	i := 0
	for v >= 1024 && i < len(units)-1 {
		v /= 1024
		i++
	}
	if i == 0 {
		return strconv.FormatInt(size, 10)
	}
	return fmt.Sprintf("%.1f%s", v, units[i])
}
