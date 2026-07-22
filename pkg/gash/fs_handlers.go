package gash

import (
	"context"
	"errors"
	"io"
	iofs "io/fs"
	"os"
	"path"
	"strings"

	gfs "github.com/rumpl/gash/pkg/fs"
	"mvdan.cc/sh/v3/interp"
)

func (b *Bash) openHandler(ctx context.Context, name string, flag int, perm os.FileMode) (io.ReadWriteCloser, error) {
	forceClobber := strings.HasPrefix(name, forceClobberPrefix)
	name = strings.TrimPrefix(name, forceClobberPrefix)
	name = handlerPath(ctx, name)
	if name == "/dev/null" {
		return &nullFile{}, nil
	}
	if flag&(os.O_WRONLY|os.O_RDWR) != 0 {
		if options := shellOptionsFromContext(ctx); !forceClobber && flag&os.O_TRUNC != 0 && options != nil && options.noclobber.Load() {
			if _, err := gfs.Stat(b.FS, name); err == nil {
				return nil, &os.PathError{Op: "bash:", Path: name, Err: iofs.ErrExist}
			}
		}
		if _, ok := b.FS.(gfs.WriteFileFS); !ok {
			return nil, &os.PathError{Op: "bash:", Path: name, Err: gfs.ErrReadOnly}
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
	return gfs.ReadDir(b.FS, name)
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
