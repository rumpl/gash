package commands

import (
	"bytes"
	"context"
	"fmt"
	iofs "io/fs"
	"path"
	"strconv"
	"strings"
	"unicode/utf8"

	gfs "github.com/rumpl/gash/pkg/fs"
)

func commandChmod(_ context.Context, args []string, c *CommandContext) int {
	recursive, verbose := false, false
	for len(args) > 0 && strings.HasPrefix(args[0], "-") {
		flag := args[0]
		if flag == "--" {
			args = args[1:]
			break
		}
		if strings.Contains(flag, "R") {
			recursive = true
		}
		if strings.Contains(flag, "v") {
			verbose = true
		}
		if !strings.ContainsAny(flag, "Rv") {
			break
		}
		args = args[1:]
	}
	if len(args) < 2 {
		return report(c, "chmod", fmt.Errorf("missing operand"))
	}
	spec, targets := args[0], args[1:]
	code := 0
	for _, target := range targets {
		if err := chmodPath(c, abs(c, target), spec, recursive, verbose, target); err != nil {
			fmt.Fprintf(c.Stderr, "chmod: cannot access '%s': No such file or directory\n", target)
			code = 1
		}
	}
	return code
}

func chmodPath(c *CommandContext, name, spec string, recursive, verbose bool, display string) error {
	info, err := gfs.Stat(c.FS, name)
	if err != nil {
		return err
	}
	mode, err := parseMode(spec, info.Mode())
	if err != nil {
		return err
	}
	if err = gfs.Chmod(c.FS, name, mode); err != nil {
		return err
	}
	if verbose {
		fmt.Fprintf(c.Stdout, "mode of '%s' changed to %04o\n", display, mode)
	}
	if recursive && info.IsDir() {
		entries, err := gfs.ReadDir(c.FS, name)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if err := chmodPath(c, path.Join(name, entry.Name()), spec, true, verbose, path.Join(display, entry.Name())); err != nil {
				return err
			}
		}
	}
	return nil
}

func parseMode(spec string, current iofs.FileMode) (iofs.FileMode, error) {
	if n, err := strconv.ParseUint(spec, 8, 32); err == nil {
		return iofs.FileMode(n), nil
	}
	mode := current.Perm()
	for _, part := range strings.Split(spec, ",") {
		i := strings.IndexAny(part, "+-=")
		if i < 0 {
			return 0, fmt.Errorf("invalid mode")
		}
		who, op, perms := part[:i], part[i], part[i+1:]
		if who == "" || strings.Contains(who, "a") {
			who = "ugo"
		}
		bits := iofs.FileMode(0)
		if strings.Contains(perms, "r") {
			bits |= 4
		}
		if strings.Contains(perms, "w") {
			bits |= 2
		}
		if strings.ContainsAny(perms, "xX") {
			bits |= 1
		}
		for _, w := range who {
			shift := 0
			if w == 'u' {
				shift = 6
			} else if w == 'g' {
				shift = 3
			} else if w != 'o' {
				return 0, fmt.Errorf("invalid mode")
			}
			mask := bits << shift
			switch op {
			case '+':
				mode |= mask
			case '-':
				mode &^= mask
			case '=':
				mode = (mode &^ (7 << shift)) | mask
			default:
				return 0, fmt.Errorf("invalid mode")
			}
		}
	}
	return mode, nil
}

func commandStat(_ context.Context, args []string, c *CommandContext) int {
	format := ""
	var files []string
	for i := 0; i < len(args); i++ {
		if args[i] == "-c" && i+1 < len(args) {
			format = args[i+1]
			i++
		} else {
			files = append(files, args[i])
		}
	}
	if len(files) == 0 {
		return report(c, "stat", fmt.Errorf("missing operand"))
	}
	code := 0
	for _, name := range files {
		info, err := gfs.Stat(c.FS, abs(c, name))
		if err != nil {
			fmt.Fprintf(c.Stderr, "stat: cannot stat '%s': No such file or directory\n", name)
			code = 1
			continue
		}
		if format != "" {
			kind := "regular file"
			if info.IsDir() {
				kind = "directory"
			}
			modeString := info.Mode().String()
			r := strings.NewReplacer("%n", name, "%N", "'"+name+"'", "%s", strconv.FormatInt(info.Size(), 10), "%F", kind, "%a", fmt.Sprintf("%o", info.Mode().Perm()), "%A", modeString, "%u", "1000", "%U", "user", "%g", "1000", "%G", "group")
			fmt.Fprintln(c.Stdout, r.Replace(format))
		} else {
			fmt.Fprintf(c.Stdout, "  File: %s\n  Size: %d\t\tBlocks: %d\nAccess: (%04o/%s)\nModify: %s\n", name, info.Size(), (info.Size()+511)/512, info.Mode().Perm(), info.Mode().String(), info.ModTime().UTC().Format("2006-01-02T15:04:05.000Z"))
		}
	}
	return code
}

var textExtensions = map[string][2]string{".go": {"Go source", "text/x-go"}, ".js": {"JavaScript source", "text/javascript"}, ".ts": {"TypeScript source", "text/typescript"}, ".py": {"Python script", "text/x-python"}, ".sh": {"Bourne-Again shell script", "text/x-shellscript"}, ".json": {"JSON data", "application/json"}, ".yaml": {"YAML data", "text/yaml"}, ".yml": {"YAML data", "text/yaml"}, ".xml": {"XML document", "application/xml"}, ".csv": {"CSV text", "text/csv"}, ".html": {"HTML document", "text/html"}, ".css": {"CSS stylesheet", "text/css"}, ".md": {"Markdown document", "text/markdown"}, ".txt": {"ASCII text", "text/plain"}}

func commandFile(_ context.Context, args []string, c *CommandContext) int {
	brief, mime, deref := false, false, false
	var files []string
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") && arg != "-" {
			brief = brief || strings.Contains(arg, "b")
			mime = mime || strings.Contains(arg, "i")
			deref = deref || strings.Contains(arg, "L")
		} else {
			files = append(files, arg)
		}
	}
	if len(files) == 0 {
		return report(c, "file", fmt.Errorf("usage: file [-bLi] FILE..."))
	}
	code := 0
	for _, name := range files {
		full := abs(c, name)
		info, err := gfs.Lstat(c.FS, full)
		if deref {
			info, err = gfs.Stat(c.FS, full)
		}
		desc := ""
		if err != nil {
			desc = "cannot open (No such file or directory)"
			code = 1
		} else if info.Mode()&iofs.ModeSymlink != 0 {
			target, _ := gfs.Readlink(c.FS, full)
			if mime {
				desc = "inode/symlink"
			} else {
				desc = "symbolic link to " + target
			}
		} else if info.IsDir() {
			if mime {
				desc = "inode/directory"
			} else {
				desc = "directory"
			}
		} else {
			data, e := gfs.ReadFile(c.FS, full)
			if e != nil {
				desc = "cannot open"
				code = 1
			} else {
				desc = detectFile(name, data, mime)
			}
		}
		if brief {
			fmt.Fprintln(c.Stdout, desc)
		} else {
			fmt.Fprintf(c.Stdout, "%s: %s\n", name, desc)
		}
	}
	return code
}

func detectFile(name string, data []byte, mime bool) string {
	if len(data) == 0 {
		if mime {
			return "inode/x-empty"
		}
		return "empty"
	}
	magicDesc, magicMime := "", ""
	switch {
	case bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")):
		magicDesc, magicMime = "PNG image data", "image/png"
	case bytes.HasPrefix(data, []byte("\xff\xd8\xff")):
		magicDesc, magicMime = "JPEG image data", "image/jpeg"
	case bytes.HasPrefix(data, []byte("GIF8")):
		magicDesc, magicMime = "GIF image data", "image/gif"
	case bytes.HasPrefix(data, []byte("%PDF")):
		magicDesc, magicMime = "PDF document", "application/pdf"
	case bytes.HasPrefix(data, []byte("PK\x03\x04")):
		magicDesc, magicMime = "Zip archive data", "application/zip"
	case bytes.HasPrefix(data, []byte("\x1f\x8b")):
		magicDesc, magicMime = "gzip compressed data", "application/gzip"
	}
	if magicDesc != "" {
		if mime {
			return magicMime
		}
		return magicDesc
	}
	if ext, ok := textExtensions[strings.ToLower(path.Ext(name))]; ok {
		if mime {
			return ext[1]
		}
		return ext[0]
	}
	if bytes.IndexByte(data, 0) >= 0 || !utf8.Valid(data) {
		if mime {
			return "application/octet-stream"
		}
		return "data"
	}
	if mime {
		return "text/plain"
	}
	for _, b := range data {
		if b >= 128 {
			return "UTF-8 Unicode text"
		}
	}
	return "ASCII text"
}
