package files

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	iofs "io/fs"
	"path"
	"strings"

	"github.com/rumpl/gash/internal/commandhelp"
	gfs "github.com/rumpl/gash/pkg/fs"
)

const mktempAttempts = 100

var mktempRandom io.Reader = rand.Reader

var mktempHelp = commandhelp.Info{
	Name:    "mktemp",
	Summary: "create a temporary file or directory",
	Usage:   "mktemp [OPTION]... [TEMPLATE]",
	Description: []string{
		"Create an empty file using TEMPLATE, whose final component must end in at least three X characters. With no TEMPLATE, use tmp.XXXXXXXXXX in the current directory.",
	},
	Options: []string{
		"-d                 create a directory instead of a file",
		"-p DIR             create in DIR; TEMPLATE, if supplied, must be a basename",
		"    --tmpdir=DIR    create in DIR; TEMPLATE, if supplied, must be a basename",
		"    --help          display this help and exit",
		"    --              end option parsing",
	},
	Notes: []string{
		"Only -d, -p, --tmpdir=DIR, --help, and -- are supported. Flags such as -u, -q, ownership, and suffix options are rejected.",
		"Creation uses only the configured virtual filesystem and retries collisions a bounded number of times.",
	},
}

type mktempOptions struct {
	directory bool
	help      bool
	tmpdir    string
	template  string
}

func commandMktemp(_ context.Context, args []string, c *CommandContext) int {
	opts, ok := parseMktemp(args, c)
	if !ok {
		return 1
	}
	if opts.help {
		return commandhelp.Show(c, mktempHelp)
	}
	template, display, ok := mktempTemplate(opts, c)
	if !ok {
		return 1
	}

	xCount := len(template) - len(strings.TrimRight(template, "X"))
	if xCount < 3 {
		fmt.Fprintln(c.Stderr, "mktemp: template must end with at least 3 consecutive X characters")
		return 1
	}
	prefix := template[:len(template)-xCount]
	displayPrefix := display[:len(display)-xCount]

	for attempt := 0; attempt < mktempAttempts; attempt++ {
		suffix, err := randomAlphaNumeric(xCount)
		if err != nil {
			return report(c, "mktemp", err)
		}
		name := prefix + suffix
		if _, err := gfs.Lstat(c.FS, name); err == nil {
			continue
		} else if !errors.Is(err, iofs.ErrNotExist) {
			return report(c, "mktemp: "+name, err)
		}

		if opts.directory {
			err = gfs.Mkdir(c.FS, name, c.CreationMode(0o700))
		} else {
			err = gfs.CreateFile(c.FS, name, nil, c.CreationMode(0o600))
		}
		if errors.Is(err, iofs.ErrExist) {
			continue
		}
		if err != nil {
			return report(c, "mktemp: "+name, err)
		}
		fmt.Fprintln(c.Stdout, displayPrefix+suffix)
		return 0
	}
	fmt.Fprintf(c.Stderr, "mktemp: failed to create a unique name after %d attempts\n", mktempAttempts)
	return 1
}

func parseMktemp(args []string, c *CommandContext) (mktempOptions, bool) {
	var opts mktempOptions
	options := true
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if options && arg == "--" {
			options = false
			continue
		}
		if options && arg == "--help" {
			opts.help = true
			continue
		}
		if options && arg == "-d" {
			opts.directory = true
			continue
		}
		if options && arg == "-p" {
			if opts.tmpdir != "" {
				fmt.Fprintln(c.Stderr, "mktemp: temporary directory specified more than once")
				return opts, false
			}
			i++
			if i == len(args) {
				fmt.Fprintln(c.Stderr, "mktemp: option requires an argument -- 'p'")
				return opts, false
			}
			opts.tmpdir = args[i]
			continue
		}
		if options && strings.HasPrefix(arg, "--tmpdir=") {
			if opts.tmpdir != "" {
				fmt.Fprintln(c.Stderr, "mktemp: temporary directory specified more than once")
				return opts, false
			}
			opts.tmpdir = strings.TrimPrefix(arg, "--tmpdir=")
			if opts.tmpdir == "" {
				fmt.Fprintln(c.Stderr, "mktemp: --tmpdir requires a non-empty directory")
				return opts, false
			}
			continue
		}
		if options && strings.HasPrefix(arg, "-") && arg != "-" {
			commandhelp.UnknownOption(c, "mktemp", arg)
			return opts, false
		}
		if opts.template != "" {
			fmt.Fprintln(c.Stderr, "mktemp: too many templates")
			return opts, false
		}
		opts.template = arg
	}
	return opts, true
}

func mktempTemplate(opts mktempOptions, c *CommandContext) (absoluteTemplate, display string, ok bool) {
	template := opts.template
	if template == "" {
		template = "tmp.XXXXXXXXXX"
	}
	if opts.tmpdir != "" {
		if opts.template != "" && path.Base(opts.template) != opts.template {
			fmt.Fprintln(c.Stderr, "mktemp: template must be a basename when used with -p or --tmpdir")
			return "", "", false
		}
		template = path.Join(opts.tmpdir, template)
	}
	absoluteInput := strings.HasPrefix(template, "/")
	absoluteTemplate = abs(c, path.Clean(template))
	if absoluteInput {
		return absoluteTemplate, absoluteTemplate, true
	}
	return absoluteTemplate, mktempDisplayPath(*c.Cwd, absoluteTemplate), true
}

// mktempDisplayPath keeps ordinary paths relative to cwd, but prints an
// absolute virtual path when reaching the created entry would require parent
// traversal. This makes the reported path normalized and directly reusable
// without preserving ambiguous . or .. components from the input.
func mktempDisplayPath(cwd, absoluteTemplate string) string {
	cwd = path.Clean(cwd)
	absoluteTemplate = path.Clean(absoluteTemplate)
	if cwd == "/" {
		return strings.TrimPrefix(absoluteTemplate, "/")
	}
	if strings.HasPrefix(absoluteTemplate, cwd+"/") {
		return strings.TrimPrefix(absoluteTemplate, cwd+"/")
	}
	return absoluteTemplate
}

func randomAlphaNumeric(length int) (string, error) {
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	bytes := make([]byte, length)
	if _, err := io.ReadFull(mktempRandom, bytes); err != nil {
		return "", err
	}
	for i := range bytes {
		bytes[i] = alphabet[int(bytes[i])%len(alphabet)]
	}
	return string(bytes), nil
}
