package files

import (
	"context"
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"

	gfs "github.com/rumpl/gash/pkg/fs"
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
