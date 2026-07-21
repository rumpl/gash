package files

import (
	"bytes"
	"context"
	"fmt"
	iofs "io/fs"
	"path"
	"strings"
	"unicode/utf8"

	gfs "github.com/rumpl/gash/pkg/fs"
)

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
