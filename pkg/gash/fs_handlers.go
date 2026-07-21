package gash

import (
	"context"
	"errors"
	"io"
	iofs "io/fs"
	"os"
	"path"
	"strings"
	"time"

	gfs "github.com/rumpl/gash/pkg/fs"
	"mvdan.cc/sh/v3/interp"
)

func (b *Bash) openHandler(ctx context.Context, name string, flag int, perm os.FileMode) (io.ReadWriteCloser, error) {
	name = handlerPath(ctx, name)
	if name == "/dev/null" {
		return &nullFile{}, nil
	}
	if flag&(os.O_WRONLY|os.O_RDWR) != 0 {
		if _, ok := b.FS.(gfs.WriteFileFS); !ok {
			return nil, gfs.ErrReadOnly
		}
	}
	var initial []byte
	if flag&os.O_WRONLY == 0 || flag&os.O_APPEND != 0 {
		data, err := gfs.ReadFile(b.FS, name)
		if err == nil {
			initial = data
		} else if flag&os.O_CREATE == 0 {
			return nil, err
		}
	}
	if flag&os.O_EXCL != 0 {
		if _, err := gfs.Stat(b.FS, name); err == nil {
			return nil, iofs.ErrExist
		}
	}
	vf := &virtualFile{fs: b.FS, name: name, perm: perm, write: flag&(os.O_WRONLY|os.O_RDWR) != 0, appendMode: flag&os.O_APPEND != 0, data: append([]byte(nil), initial...)}
	if flag&os.O_TRUNC != 0 {
		vf.data = nil
	}
	if vf.appendMode {
		vf.offset = len(vf.data)
	}
	return vf, nil
}

func (b *Bash) readDirHandler(ctx context.Context, name string) ([]iofs.DirEntry, error) {
	name = handlerPath(ctx, name)
	entries, err := gfs.ReadDir(b.FS, name)
	if err != nil {
		return nil, err
	}
	if name == "/bin" || name == "/usr/bin" {
		seen := map[string]bool{}
		for _, entry := range entries {
			seen[entry.Name()] = true
		}
		for command := range b.commands {
			if !seen[command] {
				entries = append(entries, syntheticEntry(command))
			}
		}
	}
	return entries, nil
}

func (b *Bash) statHandler(ctx context.Context, name string, follow bool) (iofs.FileInfo, error) {
	name = handlerPath(ctx, name)
	var info iofs.FileInfo
	var err error
	if follow {
		info, err = gfs.Stat(b.FS, name)
	} else {
		info, err = gfs.Lstat(b.FS, name)
	}
	if err != nil && (name == "/bin" || name == "/usr/bin") {
		return syntheticDirInfo(path.Base(name)), nil
	}
	if err != nil && (path.Dir(name) == "/bin" || path.Dir(name) == "/usr/bin") {
		base := path.Base(name)
		if _, ok := b.commands[base]; ok {
			return syntheticInfo(base), nil
		}
		if base == "bash" || base == "sh" {
			return syntheticInfo(base), nil
		}
	}
	return info, err
}

func handlerPath(ctx context.Context, name string) (result string) {
	if strings.HasPrefix(name, "/") {
		return path.Clean(name)
	}
	// Some interpreter-internal filesystem calls don't install HandlerContext.
	// They already pass cwd-resolved names; recover only guards that distinction.
	defer func() {
		if recover() != nil {
			result = path.Clean("/" + name)
		}
	}()
	return resolve(interp.HandlerCtx(ctx).Dir, name)
}

type syntheticInfo string

func (i syntheticInfo) Name() string {
	return string(i)
}

func (syntheticInfo) Size() int64 {
	return 0
}

func (syntheticInfo) Mode() iofs.FileMode {
	return 0o755
}

func (syntheticInfo) ModTime() time.Time {
	return time.Time{}
}

func (syntheticInfo) IsDir() bool {
	return false
}

func (syntheticInfo) Sys() any {
	return nil
}

type syntheticDirInfo string

func (i syntheticDirInfo) Name() string {
	return string(i)
}

func (syntheticDirInfo) Size() int64 {
	return 0
}

func (syntheticDirInfo) Mode() iofs.FileMode {
	return iofs.ModeDir | 0o755
}

func (syntheticDirInfo) ModTime() time.Time {
	return time.Time{}
}

func (syntheticDirInfo) IsDir() bool {
	return true
}

func (syntheticDirInfo) Sys() any {
	return nil
}

type syntheticEntry string

func (e syntheticEntry) Name() string {
	return string(e)
}

func (syntheticEntry) IsDir() bool {
	return false
}

func (syntheticEntry) Type() iofs.FileMode {
	return 0
}

func (e syntheticEntry) Info() (iofs.FileInfo, error) {
	return syntheticInfo(e), nil
}

type virtualFile struct {
	fs                iofs.FS
	name              string
	perm              os.FileMode
	data              []byte
	offset            int
	write, appendMode bool
}

func (f *virtualFile) Read(p []byte) (int, error) {
	if f.offset >= len(f.data) {
		return 0, io.EOF
	}
	n := copy(p, f.data[f.offset:])
	f.offset += n
	return n, nil
}

func (f *virtualFile) Write(p []byte) (int, error) {
	if !f.write {
		return 0, errors.New("file not open for writing")
	}
	end := f.offset + len(p)
	if end > len(f.data) {
		f.data = append(f.data, make([]byte, end-len(f.data))...)
	}
	copy(f.data[f.offset:end], p)
	f.offset = end
	return len(p), nil
}

func (f *virtualFile) Close() error {
	if !f.write {
		return nil
	}
	return gfs.WriteFile(f.fs, f.name, f.data, f.perm)
}

type nullFile struct{}

func (*nullFile) Read([]byte) (int, error) {
	return 0, io.EOF
}

func (*nullFile) Write(p []byte) (int, error) {
	return len(p), nil
}

func (*nullFile) Close() error {
	return nil
}

func (b *Bash) ReadFile(name string) (string, error) {
	data, e := gfs.ReadFile(b.FS, resolve(b.cwd, name))
	return string(data), e
}

func (b *Bash) WriteFile(name, data string) error {
	return gfs.WriteFile(b.FS, resolve(b.cwd, name), []byte(data), 0o644)
}

func (b *Bash) GetCwd() string {
	return b.cwd
}

func (b *Bash) GetEnv() map[string]string {
	return cloneMap(b.env)
}
