package files

import (
	"context"
	"fmt"
	iofs "io/fs"
	"path"
	"strings"

	"github.com/rumpl/gash/internal/commandhelp"
	gfs "github.com/rumpl/gash/pkg/fs"
)

func commandInstall(_ context.Context, args []string, c *CommandContext) int {
	directory, createParents := false, false
	modeSpec := ""
	operands := make([]string, 0, len(args))
	options := true

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if options && arg == "--" {
			options = false
			continue
		}
		if !options || arg == "-" || !strings.HasPrefix(arg, "-") {
			operands = append(operands, arg)
			continue
		}
		switch {
		case arg == "--help":
			return commandhelp.Show(c, installHelp)
		case arg == "-D":
			createParents = true
		case arg == "-d" || arg == "--directory":
			directory = true
		case arg == "-m" || arg == "--mode":
			if i+1 >= len(args) {
				return report(c, "install", fmt.Errorf("option '%s' requires an argument", arg))
			}
			i++
			modeSpec = args[i]
		case strings.HasPrefix(arg, "--mode="):
			modeSpec = strings.TrimPrefix(arg, "--mode=")
			if modeSpec == "" {
				return report(c, "install", fmt.Errorf("option '--mode' requires an argument"))
			}
		case strings.HasPrefix(arg, "-m") && len(arg) > 2:
			modeSpec = arg[2:]
		case arg == "-o" || arg == "-g":
			return unsupportedInstallOption(c, arg, "ownership and group changes")
		case arg == "-s" || arg == "--strip":
			return unsupportedInstallOption(c, arg, "stripping")
		case arg == "-p" || arg == "--preserve-timestamps":
			return unsupportedInstallOption(c, arg, "timestamp preservation")
		case arg == "-Z" || strings.HasPrefix(arg, "--context"):
			return unsupportedInstallOption(c, arg, "SELinux contexts")
		default:
			return commandhelp.UnknownOption(c, "install", arg)
		}
	}

	baseMode := iofs.FileMode(0o755)
	if modeSpec != "" {
		var err error
		baseMode, err = parseMode(modeSpec, baseMode)
		if err != nil {
			return report(c, "install", fmt.Errorf("invalid mode '%s'", modeSpec))
		}
	}

	if directory {
		if createParents {
			return report(c, "install", fmt.Errorf("options -d and -D are mutually exclusive"))
		}
		if len(operands) == 0 {
			return report(c, "install", fmt.Errorf("missing operand"))
		}
		code := 0
		for _, operand := range operands {
			if err := gfs.MkdirAll(c.FS, abs(c, operand), c.CreationMode(baseMode)); err != nil {
				fmt.Fprintf(c.Stderr, "install: cannot create directory '%s': %v\n", operand, err)
				code = 1
			}
		}
		return code
	}

	if len(operands) < 2 {
		return report(c, "install", fmt.Errorf("missing destination file operand"))
	}
	destArg := operands[len(operands)-1]
	sources := operands[:len(operands)-1]
	dest := abs(c, destArg)
	destInfo, destErr := gfs.Stat(c.FS, dest)
	destIsDir := destErr == nil && destInfo.IsDir()
	if len(sources) > 1 && !destIsDir {
		return report(c, "install", fmt.Errorf("target '%s' is not a directory", destArg))
	}
	if createParents && len(sources) != 1 {
		return report(c, "install", fmt.Errorf("option -D requires exactly one source and one destination"))
	}

	code := 0
	for _, srcArg := range sources {
		target := dest
		if destIsDir && !createParents {
			target = path.Join(dest, path.Base(abs(c, srcArg)))
		}
		if createParents {
			if err := gfs.MkdirAll(c.FS, path.Dir(target), c.CreationMode(0o755)); err != nil {
				fmt.Fprintf(c.Stderr, "install: cannot create parent directories for '%s': %v\n", destArg, err)
				code = 1
				continue
			}
		}
		if err := installFile(c, abs(c, srcArg), target, c.CreationMode(baseMode)); err != nil {
			fmt.Fprintf(c.Stderr, "install: cannot install '%s': %v\n", srcArg, err)
			code = 1
		}
	}
	return code
}

func installFile(c *CommandContext, src, dest string, mode iofs.FileMode) error {
	info, err := gfs.Stat(c.FS, src)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("source is not a regular file")
	}
	if src == dest {
		return fmt.Errorf("source and destination are the same file")
	}
	contents, err := gfs.ReadFile(c.FS, src)
	if err != nil {
		return err
	}
	_, destErr := gfs.Stat(c.FS, dest)
	if err := gfs.WriteFile(c.FS, dest, contents, mode); err != nil {
		return err
	}
	// WriteFile creation permissions do not necessarily change an existing
	// file's mode (for example on rooted host filesystems).
	if destErr == nil {
		return gfs.Chmod(c.FS, dest, mode)
	}
	return nil
}

func unsupportedInstallOption(c *CommandContext, option, feature string) int {
	return report(c, "install", fmt.Errorf("option '%s' is not supported (%s are unavailable)", option, feature))
}

var installHelp = commandhelp.Info{
	Name:    "install",
	Summary: "copy files and set attributes",
	Usage:   "install [OPTION]... SOURCE... DEST\ninstall -d [OPTION]... DIRECTORY...",
	Description: []string{
		"Copies regular files inside the virtual filesystem using mode 0755 by default.",
	},
	Options: []string{
		"-D                 create missing destination parent directories",
		"-d, --directory    create directories and parents instead of copying",
		"-m, --mode=MODE    set octal or supported symbolic permission mode",
		"    --              end option parsing",
		"    --help          display this help and exit",
	},
	Notes: []string{
		"Ownership/group, stripping, SELinux contexts, and timestamp preservation are explicitly unsupported.",
	},
}
