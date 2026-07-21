package files

import (
	"context"
	"fmt"
	"path"
	"strings"

	gfs "github.com/rumpl/gash/pkg/fs"
)

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
