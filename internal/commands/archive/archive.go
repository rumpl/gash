package archive

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	iofs "io/fs"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/rumpl/gash/internal/command"
	"github.com/rumpl/gash/internal/commandutil"
	gfs "github.com/rumpl/gash/pkg/fs"
)

const (
	maxArchiveInputBytes     = 512 << 20
	maxArchiveExpandedBytes  = 512 << 20
	maxArchiveEntries        = 100000
	defaultTarOutputFileMode = 0o644
)

type CommandContext = command.Context

type limitReader struct {
	r         io.Reader
	remaining int64
}

func (r *limitReader) Read(p []byte) (int, error) {
	if r.remaining <= 0 {
		return 0, fmt.Errorf("archive input limit exceeded (%d bytes)", maxArchiveInputBytes)
	}
	if int64(len(p)) > r.remaining {
		p = p[:r.remaining]
	}
	n, err := r.r.Read(p)
	r.remaining -= int64(n)
	return n, err
}

func readAllLimited(r io.Reader, limit int64, label string) ([]byte, error) {
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, io.LimitReader(r, limit+1)); err != nil {
		return nil, err
	}
	if int64(buf.Len()) > limit {
		return nil, fmt.Errorf("%s limit exceeded (%d bytes)", label, limit)
	}
	return buf.Bytes(), nil
}

func report(ctx *CommandContext, name string, err error) int {
	return commandutil.Report(ctx, name, err)
}

func abs(ctx *CommandContext, name string) string {
	return commandutil.Abs(ctx, name)
}

func writeFile(ctx *CommandContext, name string, data []byte, perm iofs.FileMode) error {
	if err := gfs.MkdirAll(ctx.FS, path.Dir(name), 0o755); err != nil {
		return err
	}
	return gfs.WriteFile(ctx.FS, name, data, perm)
}

func stripGzipSuffix(name string) string {
	for _, suffix := range []string{".gz", ".tgz", "-gz", "_gz", ".Z"} {
		if strings.HasSuffix(name, suffix) {
			if suffix == ".tgz" {
				return strings.TrimSuffix(name, suffix) + ".tar"
			}
			return strings.TrimSuffix(name, suffix)
		}
	}
	return name + ".out"
}

func gzipDefaultOutput(name string) string {
	if name == "-" || name == "" {
		return ""
	}
	return name + ".gz"
}

func commandGzip(_ context.Context, args []string, ctx *CommandContext) int {
	return runGzip(args, ctx, false, false)
}

func commandGunzip(_ context.Context, args []string, ctx *CommandContext) int {
	return runGzip(args, ctx, true, false)
}

func commandZcat(_ context.Context, args []string, ctx *CommandContext) int {
	return runGzip(args, ctx, true, true)
}

type gzipOptions struct {
	decompress bool
	stdout     bool
	keep       bool
	force      bool
	test       bool
	level      int
	files      []string
}

func runGzip(args []string, ctx *CommandContext, defaultDecompress, forceStdout bool) int {
	opts := gzipOptions{decompress: defaultDecompress, stdout: forceStdout, level: gzip.DefaultCompression}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			opts.files = append(opts.files, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			opts.files = append(opts.files, arg)
			continue
		}
		switch {
		case arg == "-c" || arg == "--stdout" || arg == "--to-stdout":
			opts.stdout = true
		case arg == "-d" || arg == "--decompress" || arg == "--uncompress":
			opts.decompress = true
		case arg == "-k" || arg == "--keep":
			opts.keep = true
		case arg == "-f" || arg == "--force":
			opts.force = true
		case arg == "-t" || arg == "--test":
			opts.test = true
		case arg == "-n" || arg == "--no-name", arg == "-N" || arg == "--name":
			// gzip header names are not preserved by gash; accept flags for compatibility.
		case arg == "-q" || arg == "--quiet", arg == "-v" || arg == "--verbose":
			// Diagnostics are intentionally compact.
		case strings.HasPrefix(arg, "-") && len(arg) == 2 && arg[1] >= '1' && arg[1] <= '9':
			opts.level = int(arg[1] - '0')
		case arg == "-0":
			opts.level = gzip.NoCompression
		default:
			fmt.Fprintf(ctx.Stderr, "gzip: unsupported option %s\n", arg)
			return 1
		}
	}
	if len(opts.files) == 0 {
		opts.files = []string{"-"}
		opts.stdout = true
	}
	code := 0
	for _, name := range opts.files {
		if err := gzipOne(ctx, opts, name); err != nil {
			fmt.Fprintf(ctx.Stderr, "gzip: %s: %v\n", name, err)
			code = 1
		}
	}
	return code
}

func gzipOne(ctx *CommandContext, opts gzipOptions, operand string) error {
	var input []byte
	var err error
	if operand == "-" {
		input, err = readAllLimited(ctx.Stdin, maxArchiveInputBytes, "gzip input")
	} else {
		input, err = gfs.ReadFile(ctx.FS, abs(ctx, operand))
		if err == nil && int64(len(input)) > maxArchiveInputBytes {
			err = fmt.Errorf("gzip input limit exceeded (%d bytes)", maxArchiveInputBytes)
		}
	}
	if err != nil {
		return err
	}
	if opts.decompress {
		zr, err := gzip.NewReader(bytes.NewReader(input))
		if err != nil {
			return err
		}
		decompressed, err := readAllLimited(zr, maxArchiveExpandedBytes, "gzip decompressed output")
		closeErr := zr.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		if opts.test {
			return nil
		}
		if opts.stdout {
			_, err = ctx.Stdout.Write(decompressed)
			return err
		}
		out := stripGzipSuffix(operand)
		if out == operand {
			return fmt.Errorf("unknown suffix -- ignored")
		}
		return writeFile(ctx, abs(ctx, out), decompressed, defaultTarOutputFileMode)
	}
	var out bytes.Buffer
	zw, err := gzip.NewWriterLevel(&out, opts.level)
	if err != nil {
		return err
	}
	if _, err := zw.Write(input); err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}
	if opts.stdout {
		_, err = ctx.Stdout.Write(out.Bytes())
		return err
	}
	return writeFile(ctx, abs(ctx, gzipDefaultOutput(operand)), out.Bytes(), defaultTarOutputFileMode)
}

type tarOptions struct {
	mode        rune
	file        string
	gzip        bool
	verbose     bool
	chdir       string
	strip       int
	toStdout    bool
	noOverwrite bool
	operands    []string
}

func commandTar(_ context.Context, args []string, ctx *CommandContext) int {
	opts, err := parseTarOptions(args)
	if err != nil {
		return report(ctx, "tar", err)
	}
	switch opts.mode {
	case 'c':
		return tarCreate(ctx, opts)
	case 't':
		return tarList(ctx, opts)
	case 'x':
		return tarExtract(ctx, opts)
	default:
		return report(ctx, "tar", errors.New("must specify one of -c, -t, or -x"))
	}
}

func parseTarOptions(args []string) (tarOptions, error) {
	opts := tarOptions{chdir: ""}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			opts.operands = append(opts.operands, args[i+1:]...)
			break
		}
		if arg == "-" || !strings.HasPrefix(arg, "-") {
			opts.operands = append(opts.operands, arg)
			continue
		}
		if strings.HasPrefix(arg, "--file=") {
			opts.file = strings.TrimPrefix(arg, "--file=")
			continue
		}
		if strings.HasPrefix(arg, "--strip-components=") {
			n, err := strconv.Atoi(strings.TrimPrefix(arg, "--strip-components="))
			if err != nil || n < 0 {
				return opts, fmt.Errorf("invalid strip-components")
			}
			opts.strip = n
			continue
		}
		switch arg {
		case "-C", "--directory":
			i++
			if i >= len(args) {
				return opts, fmt.Errorf("option %s requires an argument", arg)
			}
			opts.chdir = args[i]
			continue
		case "-f", "--file":
			i++
			if i >= len(args) {
				return opts, fmt.Errorf("option %s requires an argument", arg)
			}
			opts.file = args[i]
			continue
		case "--to-stdout", "-O":
			opts.toStdout = true
			continue
		case "--no-overwrite-dir", "--skip-old-files", "-k":
			opts.noOverwrite = true
			continue
		}
		letters := strings.TrimPrefix(arg, "-")
		for j := 0; j < len(letters); j++ {
			switch letters[j] {
			case 'c', 't', 'x':
				if opts.mode != 0 && opts.mode != rune(letters[j]) {
					return opts, fmt.Errorf("multiple archive modes specified")
				}
				opts.mode = rune(letters[j])
			case 'z':
				opts.gzip = true
			case 'v':
				opts.verbose = true
			case 'f':
				if j != len(letters)-1 {
					opts.file = letters[j+1:]
					j = len(letters)
				} else {
					i++
					if i >= len(args) {
						return opts, fmt.Errorf("option -f requires an argument")
					}
					opts.file = args[i]
				}
			case 'C':
				i++
				if i >= len(args) {
					return opts, fmt.Errorf("option -C requires an argument")
				}
				opts.chdir = args[i]
			case 'O':
				opts.toStdout = true
			default:
				return opts, fmt.Errorf("unsupported option -%c", letters[j])
			}
		}
	}
	return opts, nil
}

func tarCreate(ctx *CommandContext, opts tarOptions) int {
	base := *ctx.Cwd
	if opts.chdir != "" {
		base = commandutil.Resolve(base, opts.chdir)
	}
	var out bytes.Buffer
	tw := tar.NewWriter(&out)
	entries := 0
	for _, operand := range opts.operands {
		if operand == "-" {
			return report(ctx, "tar", errors.New("cannot archive stdin operand"))
		}
		if err := addTarPath(ctx, tw, base, path.Clean(operand), &entries, opts.verbose); err != nil {
			tw.Close()
			return report(ctx, "tar", err)
		}
	}
	if err := tw.Close(); err != nil {
		return report(ctx, "tar", err)
	}
	data := out.Bytes()
	if opts.gzip {
		var gz bytes.Buffer
		zw := gzip.NewWriter(&gz)
		if _, err := zw.Write(data); err != nil {
			return report(ctx, "tar", err)
		}
		if err := zw.Close(); err != nil {
			return report(ctx, "tar", err)
		}
		data = gz.Bytes()
	}
	if int64(len(data)) > maxArchiveExpandedBytes {
		return report(ctx, "tar", fmt.Errorf("archive output limit exceeded (%d bytes)", maxArchiveExpandedBytes))
	}
	if opts.file == "" || opts.file == "-" {
		_, err := ctx.Stdout.Write(data)
		if err != nil {
			return report(ctx, "tar", err)
		}
		return 0
	}
	if err := writeFile(ctx, abs(ctx, opts.file), data, defaultTarOutputFileMode); err != nil {
		return report(ctx, "tar", err)
	}
	return 0
}

func addTarPath(ctx *CommandContext, tw *tar.Writer, base, rel string, entries *int, verbose bool) error {
	if rel == "." || rel == "" {
		rel = "."
	}
	full := commandutil.Resolve(base, rel)
	info, err := gfs.Lstat(ctx.FS, full)
	if err != nil {
		return fmt.Errorf("%s: %w", rel, err)
	}
	if *entries >= maxArchiveEntries {
		return fmt.Errorf("archive entry limit exceeded (%d)", maxArchiveEntries)
	}
	*entries++
	name := strings.TrimPrefix(path.Clean(rel), "/")
	if name == "." {
		name = path.Base(full)
	}
	if info.IsDir() && !strings.HasSuffix(name, "/") {
		name += "/"
	}
	hdr := &tar.Header{Name: name, Mode: int64(info.Mode().Perm()), ModTime: info.ModTime(), Size: info.Size()}
	switch {
	case info.Mode()&iofs.ModeSymlink != 0:
		target, err := gfs.Readlink(ctx.FS, full)
		if err != nil {
			return err
		}
		hdr.Typeflag = tar.TypeSymlink
		hdr.Linkname = target
		hdr.Size = 0
	case info.IsDir():
		hdr.Typeflag = tar.TypeDir
		hdr.Size = 0
	case info.Mode().IsRegular():
		hdr.Typeflag = tar.TypeReg
	default:
		return fmt.Errorf("%s: unsupported file type", rel)
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	if verbose {
		fmt.Fprintln(ctx.Stderr, strings.TrimSuffix(name, "/"))
	}
	if info.Mode().IsRegular() {
		data, err := gfs.ReadFile(ctx.FS, full)
		if err != nil {
			return err
		}
		if _, err := tw.Write(data); err != nil {
			return err
		}
	}
	if info.IsDir() {
		children, err := gfs.ReadDir(ctx.FS, full)
		if err != nil {
			return err
		}
		sort.Slice(children, func(i, j int) bool { return children[i].Name() < children[j].Name() })
		for _, child := range children {
			childRel := path.Join(rel, child.Name())
			if rel == "." {
				childRel = child.Name()
			}
			if err := addTarPath(ctx, tw, base, childRel, entries, verbose); err != nil {
				return err
			}
		}
	}
	return nil
}

func tarReader(ctx *CommandContext, opts tarOptions) (*tar.Reader, io.Closer, error) {
	var r io.Reader
	if opts.file == "" || opts.file == "-" {
		r = ctx.Stdin
	} else {
		data, err := gfs.ReadFile(ctx.FS, abs(ctx, opts.file))
		if err != nil {
			return nil, nil, err
		}
		r = bytes.NewReader(data)
	}
	r = &limitReader{r: r, remaining: maxArchiveInputBytes}
	var closer io.Closer = io.NopCloser(bytes.NewReader(nil))
	if opts.gzip || strings.HasSuffix(opts.file, ".tgz") || strings.HasSuffix(opts.file, ".tar.gz") {
		zr, err := gzip.NewReader(r)
		if err != nil {
			return nil, nil, err
		}
		closer = zr
		r = zr
	}
	return tar.NewReader(r), closer, nil
}

func tarList(ctx *CommandContext, opts tarOptions) int {
	tr, closer, err := tarReader(ctx, opts)
	if err != nil {
		return report(ctx, "tar", err)
	}
	defer closer.Close()
	entries := 0
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return report(ctx, "tar", err)
		}
		entries++
		if entries > maxArchiveEntries {
			return report(ctx, "tar", fmt.Errorf("archive entry limit exceeded (%d)", maxArchiveEntries))
		}
		if !tarNameWanted(opts, hdr.Name) {
			continue
		}
		fmt.Fprintln(ctx.Stdout, hdr.Name)
	}
	return 0
}

func tarExtract(ctx *CommandContext, opts tarOptions) int {
	tr, closer, err := tarReader(ctx, opts)
	if err != nil {
		return report(ctx, "tar", err)
	}
	defer closer.Close()
	destBase := *ctx.Cwd
	if opts.chdir != "" {
		destBase = commandutil.Resolve(destBase, opts.chdir)
	}
	entries := 0
	var expanded int64
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return report(ctx, "tar", err)
		}
		entries++
		if entries > maxArchiveEntries {
			return report(ctx, "tar", fmt.Errorf("archive entry limit exceeded (%d)", maxArchiveEntries))
		}
		if !tarNameWanted(opts, hdr.Name) {
			continue
		}
		name, ok := safeTarName(hdr.Name, opts.strip)
		if !ok {
			return report(ctx, "tar", fmt.Errorf("refusing unsafe archive path %q", hdr.Name))
		}
		if name == "" {
			continue
		}
		dest := commandutil.Resolve(destBase, name)
		if !within(destBase, dest) {
			return report(ctx, "tar", fmt.Errorf("refusing archive path outside destination %q", hdr.Name))
		}
		if hdr.Size > 0 {
			expanded += hdr.Size
			if expanded > maxArchiveExpandedBytes {
				return report(ctx, "tar", fmt.Errorf("archive expanded output limit exceeded (%d bytes)", maxArchiveExpandedBytes))
			}
		}
		if opts.verbose {
			fmt.Fprintln(ctx.Stderr, name)
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := gfs.MkdirAll(ctx.FS, dest, iofs.FileMode(hdr.Mode).Perm()); err != nil {
				return report(ctx, "tar", err)
			}
		case tar.TypeReg, tar.TypeRegA:
			data, err := readAllLimited(tr, maxArchiveExpandedBytes-expanded+hdr.Size, "archive expanded output")
			if err != nil {
				return report(ctx, "tar", err)
			}
			if opts.toStdout {
				if _, err := ctx.Stdout.Write(data); err != nil {
					return report(ctx, "tar", err)
				}
				continue
			}
			if opts.noOverwrite {
				if _, err := gfs.Lstat(ctx.FS, dest); err == nil {
					continue
				}
			}
			if err := ensureNoSymlinkParent(ctx, destBase, dest); err != nil {
				return report(ctx, "tar", err)
			}
			if err := writeFile(ctx, dest, data, iofs.FileMode(hdr.Mode).Perm()); err != nil {
				return report(ctx, "tar", err)
			}
			mtime := hdr.ModTime
			if mtime.IsZero() {
				mtime = time.Now()
			}
			_ = gfs.Chtimes(ctx.FS, dest, mtime, mtime)
		case tar.TypeSymlink:
			if opts.toStdout {
				continue
			}
			if !safeLinkTarget(destBase, dest, hdr.Linkname) {
				return report(ctx, "tar", fmt.Errorf("refusing unsafe symlink %q -> %q", hdr.Name, hdr.Linkname))
			}
			if err := ensureNoSymlinkParent(ctx, destBase, dest); err != nil {
				return report(ctx, "tar", err)
			}
			if opts.noOverwrite {
				if _, err := gfs.Lstat(ctx.FS, dest); err == nil {
					continue
				}
			}
			if err := gfs.MkdirAll(ctx.FS, path.Dir(dest), 0o755); err != nil {
				return report(ctx, "tar", err)
			}
			if err := gfs.Symlink(ctx.FS, hdr.Linkname, dest); err != nil {
				return report(ctx, "tar", err)
			}
		case tar.TypeLink:
			if opts.toStdout {
				continue
			}
			linkName, ok := safeTarName(hdr.Linkname, opts.strip)
			if !ok || linkName == "" {
				return report(ctx, "tar", fmt.Errorf("refusing unsafe hardlink %q -> %q", hdr.Name, hdr.Linkname))
			}
			target := commandutil.Resolve(destBase, linkName)
			if !within(destBase, target) || !within(destBase, dest) {
				return report(ctx, "tar", fmt.Errorf("refusing unsafe hardlink %q -> %q", hdr.Name, hdr.Linkname))
			}
			if err := ensureNoSymlinkParent(ctx, destBase, dest); err != nil {
				return report(ctx, "tar", err)
			}
			if err := gfs.Link(ctx.FS, target, dest); err != nil {
				return report(ctx, "tar", err)
			}
		default:
			return report(ctx, "tar", fmt.Errorf("unsupported archive entry type %c", hdr.Typeflag))
		}
	}
	return 0
}

func tarNameWanted(opts tarOptions, name string) bool {
	if len(opts.operands) == 0 {
		return true
	}
	clean := strings.TrimPrefix(path.Clean("/"+name), "/")
	for _, op := range opts.operands {
		op = strings.TrimPrefix(path.Clean("/"+op), "/")
		if clean == op || strings.HasPrefix(clean, strings.TrimSuffix(op, "/")+"/") {
			return true
		}
	}
	return false
}

func safeTarName(name string, strip int) (string, bool) {
	if name == "" || strings.Contains(name, "\x00") || strings.HasPrefix(name, "/") || strings.HasPrefix(name, "\\") || strings.Contains(name, ":") {
		return "", false
	}
	parts := strings.Split(name, "/")
	cleanParts := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" || part == "." {
			continue
		}
		if part == ".." {
			return "", false
		}
		cleanParts = append(cleanParts, part)
	}
	if strip > len(cleanParts) {
		return "", true
	}
	cleanParts = cleanParts[strip:]
	if len(cleanParts) == 0 {
		return "", true
	}
	return path.Join(cleanParts...), true
}

func within(base, target string) bool {
	base = path.Clean(base)
	target = path.Clean(target)
	return target == base || strings.HasPrefix(target, strings.TrimSuffix(base, "/")+"/")
}

func safeLinkTarget(base, linkPath, target string) bool {
	if target == "" || strings.Contains(target, "\x00") {
		return false
	}
	var resolved string
	if strings.HasPrefix(target, "/") {
		resolved = path.Clean(target)
	} else {
		resolved = path.Clean(path.Join(path.Dir(linkPath), target))
	}
	return within(base, resolved)
}

func ensureNoSymlinkParent(ctx *CommandContext, base, dest string) error {
	base = path.Clean(base)
	dir := path.Dir(path.Clean(dest))
	if !within(base, dir) {
		return fmt.Errorf("refusing path outside destination")
	}
	if dir == base || dir == "." || dir == "/" {
		return nil
	}
	baseParts := strings.Split(strings.Trim(strings.TrimPrefix(base, "/"), "/"), "/")
	dirParts := strings.Split(strings.Trim(strings.TrimPrefix(dir, "/"), "/"), "/")
	prefixLen := len(baseParts)
	if base == "/" {
		prefixLen = 0
	}
	for i := prefixLen; i < len(dirParts); i++ {
		p := "/" + path.Join(dirParts[:i+1]...)
		info, err := gfs.Lstat(ctx.FS, p)
		if err != nil {
			if errors.Is(err, iofs.ErrNotExist) {
				continue
			}
			return err
		}
		if info.Mode()&iofs.ModeSymlink != 0 {
			return fmt.Errorf("refusing to write through symlink %q", p)
		}
	}
	return nil
}
