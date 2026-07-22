package files

import (
	"context"
	"fmt"
	iofs "io/fs"
	"strings"

	"github.com/rumpl/gash/internal/commandhelp"
	gfs "github.com/rumpl/gash/pkg/fs"
)

var lsHelp = commandhelp.Info{
	Name:    "ls",
	Summary: "list directory contents",
	Usage:   "ls [OPTION]... [FILE]...",
	Options: []string{
		"-a, --all            do not ignore entries starting with .",
		"-A, --almost-all     do not list . and ..",
		"-d, --directory      list directories themselves, not their contents",
		"-F, --classify       append indicator (one of */=>@) to entries",
		"-h, --human-readable with -l, print sizes like 1K 234M 2G etc.",
		"-l                   use a long listing format",
		"-r, --reverse        reverse order while sorting",
		"-R, --recursive      list subdirectories recursively",
		"-S                   sort by file size, largest first",
		"-t                   sort by time, newest first",
		"-1                   list one file per line",
		"    --help           display this help and exit",
	},
}

type lsItem struct {
	name            string
	entry           iofs.DirEntry
	explicitOperand bool
}

func commandLS(_ context.Context, args []string, c *CommandContext) int {
	if commandhelp.Requested(args) {
		return commandhelp.Show(c, lsHelp)
	}

	all, almostAll, directory, long := false, false, false, false
	var names []string
	options := true
	for _, argument := range args {
		if options && argument == "--" {
			options = false
			continue
		}
		if options && strings.HasPrefix(argument, "--") {
			switch argument {
			case "--all":
				all = true
			case "--almost-all":
				almostAll = true
			case "--directory":
				directory = true
			default:
				return commandhelp.UnknownOption(c, "ls", argument)
			}
			continue
		}
		if options && strings.HasPrefix(argument, "-") && argument != "-" {
			for _, option := range strings.TrimPrefix(argument, "-") {
				switch option {
				case 'a':
					all = true
				case 'A':
					almostAll = true
				case 'd':
					directory = true
				case 'l':
					long = true
				case '1':
					// Gash already emits one item per line.
				default:
					return commandhelp.UnknownOption(c, "ls", "-"+string(option))
				}
			}
			continue
		}
		names = append(names, argument)
	}
	if len(names) == 0 {
		names = []string{"."}
	}

	code := 0
	for _, name := range names {
		items, err := lsItems(c, name, directory, all && !almostAll)
		if err != nil {
			code = report(c, "ls: "+name, err)
			continue
		}
		for _, item := range items {
			if !item.explicitOperand && !all && !almostAll && strings.HasPrefix(item.name, ".") {
				continue
			}
			if long {
				info, infoErr := item.entry.Info()
				if infoErr != nil {
					code = report(c, "ls: "+item.name, infoErr)
					continue
				}
				fmt.Fprintf(c.Stdout, "%s %8d %s\n", lsMode(info), info.Size(), item.name)
				continue
			}
			fmt.Fprintln(c.Stdout, item.name)
		}
	}
	return code
}

func lsItems(c *CommandContext, name string, directory, includeDotEntries bool) ([]lsItem, error) {
	full := abs(c, name)
	if directory {
		info, err := gfs.Lstat(c.FS, full)
		if err != nil {
			return nil, err
		}
		return []lsItem{{name: name, entry: fileInfoEntry{info}, explicitOperand: true}}, nil
	}
	entries, err := gfs.ReadDir(c.FS, full)
	if err != nil {
		info, statErr := gfs.Lstat(c.FS, full)
		if statErr != nil {
			return nil, statErr
		}
		return []lsItem{{name: name, entry: fileInfoEntry{info}, explicitOperand: true}}, nil
	}
	items := make([]lsItem, 0, len(entries)+2)
	if includeDotEntries {
		current, statErr := gfs.Stat(c.FS, full)
		if statErr != nil {
			return nil, statErr
		}
		parent, statErr := gfs.Stat(c.FS, abs(c, full+"/.."))
		if statErr != nil {
			return nil, statErr
		}
		items = append(items,
			lsItem{name: ".", entry: fileInfoEntry{current}},
			lsItem{name: "..", entry: fileInfoEntry{parent}},
		)
	}
	for _, entry := range entries {
		items = append(items, lsItem{name: entry.Name(), entry: entry})
	}
	return items, nil
}

func lsMode(info iofs.FileInfo) string {
	kind := byte('-')
	switch {
	case info.IsDir():
		kind = 'd'
	case info.Mode()&iofs.ModeSymlink != 0:
		kind = 'l'
	}
	permissions := info.Mode().Perm().String()
	return string(kind) + permissions[1:]
}
