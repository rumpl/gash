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
	ErrNotExist    = iofs.ErrNotExist
	ErrExist       = iofs.ErrExist
	ErrNotDir      = errors.New("not a directory")
	ErrIsDir       = errors.New("is a directory")
	ErrNotEmpty    = errors.New("directory not empty")
	ErrReadOnly    = errors.New("filesystem is read-only")
	ErrUnsupported = errors.New("operation not supported")
	ErrQuota       = errors.New("filesystem quota exceeded")
	ErrLoop        = errors.New("too many symbolic links")
	ErrCrossDevice = errors.New("cross-device operation")
)

// FileSystem is deliberately the standard library's minimal filesystem
// interface. Read-only implementations such as os.DirFS, embed.FS and
// fstest.MapFS can be supplied directly. Additional operations are discovered
// through the capability interfaces below.
type FileSystem = iofs.FS

type CreateFileFS interface {
	CreateFile(name string, data []byte, perm iofs.FileMode) error
}
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

// GlobalSymlinkFS stores an absolute target from an enclosing virtual
// namespace without rebasing it into the filesystem's local root.
type GlobalSymlinkFS interface {
	GlobalSymlink(target, name string) error
}
type ReadlinkFS interface {
	Readlink(name string) (string, error)
}

// VirtualReadlinkFS returns link targets in the virtual namespace. It is used
// by filesystem adapters whose native link target syntax may expose a backing
// namespace, such as Rooted host filesystems.
type VirtualReadlinkFS interface {
	VirtualReadlink(name string) (string, error)
}

// ScopedVirtualReadlinkFS additionally reports whether an absolute target is
// global to an enclosing namespace or local to this filesystem's root.
type ScopedVirtualReadlinkFS interface {
	ScopedVirtualReadlink(name string) (target string, global bool, err error)
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

func ReadFile(fsys iofs.FS, name string) ([]byte, error) {
	if Name(name) == "dev/null" {
		return []byte{}, nil
	}
	return iofs.ReadFile(fsys, Name(name))
}

func ReadDir(fsys iofs.FS, name string) ([]iofs.DirEntry, error) {
	return iofs.ReadDir(fsys, Name(name))
}

func Stat(fsys iofs.FS, name string) (iofs.FileInfo, error) {
	if Name(name) == "dev/null" {
		return nullFileInfo{}, nil
	}
	return iofs.Stat(fsys, Name(name))
}

func Lstat(fsys iofs.FS, name string) (iofs.FileInfo, error) {
	name = Name(name)
	if name == "dev/null" {
		return nullFileInfo{}, nil
	}
	if f, ok := fsys.(LstatFS); ok {
		return f.Lstat(name)
	}
	return iofs.Stat(fsys, name)
}

func Readlink(fsys iofs.FS, name string) (string, error) {
	if f, ok := fsys.(ReadlinkFS); ok {
		return f.Readlink(Name(name))
	}
	return "", ErrUnsupported
}

// VirtualReadlink reads a symbolic link target represented in the virtual
// namespace rather than any backing filesystem namespace.
func VirtualReadlink(fsys iofs.FS, name string) (string, error) {
	if f, ok := fsys.(VirtualReadlinkFS); ok {
		return f.VirtualReadlink(Name(name))
	}
	return Readlink(fsys, name)
}

func CreateFile(fsys iofs.FS, name string, data []byte, perm iofs.FileMode) error {
	if Name(name) == "dev/null" {
		return iofs.ErrExist
	}
	if f, ok := fsys.(CreateFileFS); ok {
		return f.CreateFile(Name(name), data, perm)
	}
	return ErrReadOnly
}

func WriteFile(fsys iofs.FS, name string, data []byte, perm iofs.FileMode) error {
	if Name(name) == "dev/null" {
		return nil
	}
	if f, ok := fsys.(WriteFileFS); ok {
		return f.WriteFile(Name(name), data, perm)
	}
	return ErrReadOnly
}

func AppendFile(fsys iofs.FS, name string, data []byte, perm iofs.FileMode) error {
	if Name(name) == "dev/null" {
		return nil
	}
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
