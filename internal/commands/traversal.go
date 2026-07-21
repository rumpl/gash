package commands

import (
	"context"
	"fmt"
	gfs "github.com/rumpl/gash/pkg/fs"
	"path"
	"sort"
	"strconv"
	"strings"
)

func commandTree(_ context.Context, args []string, c *CommandContext) int {
	hidden, dirsOnly, full := false, false, false
	depth := -1
	var roots []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-a":
			hidden = true
		case "-d":
			dirsOnly = true
		case "-f":
			full = true
		case "-L":
			if i+1 < len(args) {
				depth, _ = strconv.Atoi(args[i+1])
				i++
			}
		default:
			roots = append(roots, args[i])
		}
	}
	if len(roots) == 0 {
		roots = []string{"."}
	}
	totalDirs, totalFiles, code := 0, 0, 0
	for _, root := range roots {
		fmt.Fprintln(c.Stdout, root)
		d, f, err := treeWalk(c, abs(c, root), "", 0, depth, hidden, dirsOnly, full)
		totalDirs += d
		totalFiles += f
		if err != nil {
			fmt.Fprintf(c.Stderr, "tree: %s: No such file or directory\n", root)
			code = 1
		}
	}
	fmt.Fprintf(c.Stdout, "\n%d director%s", totalDirs, map[bool]string{true: "y", false: "ies"}[totalDirs == 1])
	if !dirsOnly {
		fmt.Fprintf(c.Stdout, ", %d file%s", totalFiles, map[bool]string{true: "", false: "s"}[totalFiles == 1])
	}
	fmt.Fprintln(c.Stdout)
	return code
}
func treeWalk(c *CommandContext, dir, prefix string, level, maxDepth int, hidden, dirsOnly, full bool) (int, int, error) {
	if maxDepth >= 0 && level >= maxDepth {
		return 0, 0, nil
	}
	entries, err := gfs.ReadDir(c.FS, dir)
	if err != nil {
		return 0, 0, err
	}
	filtered := entries[:0]
	for _, e := range entries {
		if !hidden && strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if dirsOnly && !e.IsDir() {
			continue
		}
		filtered = append(filtered, e)
	}
	sort.Slice(filtered, func(i, j int) bool { return filtered[i].Name() < filtered[j].Name() })
	dirs, files := 0, 0
	for i, e := range filtered {
		last := i == len(filtered)-1
		connector := "|-- "
		next := prefix + "|   "
		if last {
			connector = "`-- "
			next = prefix + "    "
		}
		name := e.Name()
		child := path.Join(dir, name)
		if full {
			name = child
		}
		fmt.Fprintln(c.Stdout, prefix+connector+name)
		if e.IsDir() {
			dirs++
			d, f, _ := treeWalk(c, child, next, level+1, maxDepth, hidden, dirsOnly, full)
			dirs += d
			files += f
		} else {
			files++
		}
	}
	return dirs, files, nil
}

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

func commandRmdir(_ context.Context, args []string, c *CommandContext) int {
	parents, verbose := false, false
	var dirs []string
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			parents = parents || strings.Contains(arg, "p")
			verbose = verbose || strings.Contains(arg, "v")
		} else {
			dirs = append(dirs, arg)
		}
	}
	if len(dirs) == 0 {
		return report(c, "rmdir", fmt.Errorf("missing operand"))
	}
	code := 0
	for _, dir := range dirs {
		current, display, removed := abs(c, dir), dir, false
		for {
			info, err := gfs.Stat(c.FS, current)
			if err != nil || !info.IsDir() {
				if removed && parents {
					break
				}
				fmt.Fprintf(c.Stderr, "rmdir: failed to remove '%s': Not a directory\n", display)
				code = 1
				break
			}
			entries, _ := gfs.ReadDir(c.FS, current)
			if len(entries) > 0 {
				if removed && parents {
					break
				}
				fmt.Fprintf(c.Stderr, "rmdir: failed to remove '%s': Directory not empty\n", display)
				code = 1
				break
			}
			if err = gfs.Remove(c.FS, current); err != nil {
				code = report(c, "rmdir", err)
				break
			}
			removed = true
			if verbose {
				fmt.Fprintf(c.Stdout, "rmdir: removing directory, '%s'\n", display)
			}
			if !parents {
				break
			}
			next := path.Dir(current)
			if next == "/" || next == current {
				break
			}
			current = next
			display = path.Dir(display)
		}
	}
	return code
}
