// Package fs defines io/fs-compatible capabilities and implementations.
package fs

import (
	"errors"
	iofs "io/fs"
	"path"
	"strings"
	"time"
)

var (
	ErrNotExist = iofs.ErrNotExist
	ErrExist    = iofs.ErrExist
	ErrNotDir   = errors.New("not a directory")
	ErrIsDir    = errors.New("is a directory")
	ErrNotEmpty = errors.New("directory not empty")
	ErrReadOnly = errors.New("filesystem is read-only")
	ErrQuota    = errors.New("filesystem quota exceeded")
	ErrLoop     = errors.New("too many symbolic links")
)

// FileSystem is deliberately the standard library's minimal filesystem
// interface. Read-only implementations such as os.DirFS, embed.FS and
// fstest.MapFS can be supplied directly. Additional operations are discovered
// through the capability interfaces below.
type FileSystem = iofs.FS

type WriteFileFS interface {
	WriteFile(name string, data []byte, perm iofs.FileMode) error
}
type AppendFileFS interface {
	AppendFile(name string, data []byte, perm iofs.FileMode) error
}
type MkdirFS interface {
	Mkdir(name string, perm iofs.FileMode) error
}
type MkdirAllFS interface {
	MkdirAll(name string, perm iofs.FileMode) error
}
type (
	RemoveFS    interface{ Remove(name string) error }
	RemoveAllFS interface{ RemoveAll(name string) error }
	RenameFS    interface {
		Rename(oldName, newName string) error
	}
)
type SymlinkFS interface {
	Symlink(oldName, newName string) error
}
type ReadlinkFS interface {
	Readlink(name string) (string, error)
}
type LstatFS interface {
	Lstat(name string) (iofs.FileInfo, error)
}
type ChmodFS interface {
	Chmod(name string, mode iofs.FileMode) error
}
type LinkFS interface {
	Link(oldName, newName string) error
}
type ChtimesFS interface {
	Chtimes(name string, atime, mtime time.Time) error
}

// Name converts a virtual absolute shell path to an io/fs path.
func Name(name string) string {
	name = path.Clean("/" + strings.TrimPrefix(name, "/"))
	if name == "/" {
		return "."
	}
	return strings.TrimPrefix(name, "/")
}

func ReadFile(fsys iofs.FS, name string) ([]byte, error) { return iofs.ReadFile(fsys, Name(name)) }
func ReadDir(fsys iofs.FS, name string) ([]iofs.DirEntry, error) {
	return iofs.ReadDir(fsys, Name(name))
}
func Stat(fsys iofs.FS, name string) (iofs.FileInfo, error) { return iofs.Stat(fsys, Name(name)) }
func Lstat(fsys iofs.FS, name string) (iofs.FileInfo, error) {
	name = Name(name)
	if f, ok := fsys.(LstatFS); ok {
		return f.Lstat(name)
	}
	return iofs.Stat(fsys, name)
}

func Readlink(fsys iofs.FS, name string) (string, error) {
	if f, ok := fsys.(ReadlinkFS); ok {
		return f.Readlink(Name(name))
	}
	return "", ErrReadOnly
}

func WriteFile(fsys iofs.FS, name string, data []byte, perm iofs.FileMode) error {
	if f, ok := fsys.(WriteFileFS); ok {
		return f.WriteFile(Name(name), data, perm)
	}
	return ErrReadOnly
}

func AppendFile(fsys iofs.FS, name string, data []byte, perm iofs.FileMode) error {
	if f, ok := fsys.(AppendFileFS); ok {
		return f.AppendFile(Name(name), data, perm)
	}
	return ErrReadOnly
}

func Mkdir(fsys iofs.FS, name string, perm iofs.FileMode) error {
	if f, ok := fsys.(MkdirFS); ok {
		return f.Mkdir(Name(name), perm)
	}
	return ErrReadOnly
}

func MkdirAll(fsys iofs.FS, name string, perm iofs.FileMode) error {
	if f, ok := fsys.(MkdirAllFS); ok {
		return f.MkdirAll(Name(name), perm)
	}
	if Name(name) == "." {
		return nil
	}
	return ErrReadOnly
}

func Remove(fsys iofs.FS, name string) error {
	if f, ok := fsys.(RemoveFS); ok {
		return f.Remove(Name(name))
	}
	return ErrReadOnly
}

func RemoveAll(fsys iofs.FS, name string) error {
	if f, ok := fsys.(RemoveAllFS); ok {
		return f.RemoveAll(Name(name))
	}
	return ErrReadOnly
}

func Rename(fsys iofs.FS, oldName, newName string) error {
	if f, ok := fsys.(RenameFS); ok {
		return f.Rename(Name(oldName), Name(newName))
	}
	return ErrReadOnly
}

func Symlink(fsys iofs.FS, target, name string) error {
	if f, ok := fsys.(SymlinkFS); ok {
		return f.Symlink(target, Name(name))
	}
	return ErrReadOnly
}

func Chmod(fsys iofs.FS, name string, mode iofs.FileMode) error {
	if f, ok := fsys.(ChmodFS); ok {
		return f.Chmod(Name(name), mode)
	}
	return ErrReadOnly
}

func Link(fsys iofs.FS, oldName, newName string) error {
	if f, ok := fsys.(LinkFS); ok {
		return f.Link(Name(oldName), Name(newName))
	}
	return ErrReadOnly
}

func Chtimes(fsys iofs.FS, name string, atime, mtime time.Time) error {
	if f, ok := fsys.(ChtimesFS); ok {
		return f.Chtimes(Name(name), atime, mtime)
	}
	return ErrReadOnly
}

// Memory is a concurrency-safe, quota-bounded io/fs implementation.
