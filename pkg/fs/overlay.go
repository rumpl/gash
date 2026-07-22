package fs

import (
	"errors"
	iofs "io/fs"
	"sort"
	"time"
)

// Overlay presents a writable upper filesystem over a read-only lower
// filesystem. Reads prefer upper entries and fall back to lower entries;
// writes are always directed to upper. Deletions only affect upper entries
// because io/fs has no portable whiteout capability.
type Overlay struct {
	upper iofs.FS
	lower iofs.FS
}

type OverlayOptions struct {
	Upper iofs.FS
	Lower iofs.FS
}

func NewOverlay(options OverlayOptions) (*Overlay, error) {
	upper := options.Upper
	if upper == nil {
		upper = NewMemory(0)
	}
	lower := options.Lower
	if lower == nil {
		lower = NewMemory(0)
	}
	return &Overlay{upper: upper, lower: lower}, nil
}

func (o *Overlay) Open(name string) (iofs.File, error) {
	name = Name(name)
	if f, err := o.upper.Open(name); err == nil {
		return f, nil
	} else if !isNotExist(err) {
		return nil, err
	}
	return o.lower.Open(name)
}

func (o *Overlay) ReadFile(name string) ([]byte, error) {
	name = Name(name)
	if data, err := iofs.ReadFile(o.upper, name); err == nil {
		return data, nil
	} else if !isNotExist(err) {
		return nil, err
	}
	return iofs.ReadFile(o.lower, name)
}

func (o *Overlay) ReadDir(name string) ([]iofs.DirEntry, error) {
	name = Name(name)
	merged := map[string]iofs.DirEntry{}
	lowerEntries, lowerErr := iofs.ReadDir(o.lower, name)
	if lowerErr != nil && !isNotExist(lowerErr) {
		return nil, lowerErr
	}
	for _, entry := range lowerEntries {
		merged[entry.Name()] = entry
	}
	upperEntries, upperErr := iofs.ReadDir(o.upper, name)
	if upperErr != nil && !isNotExist(upperErr) {
		return nil, upperErr
	}
	for _, entry := range upperEntries {
		merged[entry.Name()] = entry
	}
	if lowerErr != nil && upperErr != nil && len(merged) == 0 {
		return nil, upperErr
	}
	out := make([]iofs.DirEntry, 0, len(merged))
	for _, entry := range merged {
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out, nil
}

func (o *Overlay) Stat(name string) (iofs.FileInfo, error) {
	name = Name(name)
	if info, err := iofs.Stat(o.upper, name); err == nil {
		return info, nil
	} else if !isNotExist(err) {
		return nil, err
	}
	return iofs.Stat(o.lower, name)
}

func (o *Overlay) Lstat(name string) (iofs.FileInfo, error) {
	name = Name(name)
	if info, err := Lstat(o.upper, name); err == nil {
		return info, nil
	} else if !isNotExist(err) {
		return nil, err
	}
	return Lstat(o.lower, name)
}

func (o *Overlay) WriteFile(name string, data []byte, perm iofs.FileMode) error {
	return WriteFile(o.upper, name, data, perm)
}

func (o *Overlay) AppendFile(name string, data []byte, perm iofs.FileMode) error {
	return AppendFile(o.upper, name, data, perm)
}

func (o *Overlay) Mkdir(name string, perm iofs.FileMode) error {
	return Mkdir(o.upper, name, perm)
}

func (o *Overlay) MkdirAll(name string, perm iofs.FileMode) error {
	return MkdirAll(o.upper, name, perm)
}

func (o *Overlay) Remove(name string) error {
	return Remove(o.upper, name)
}

func (o *Overlay) RemoveAll(name string) error {
	return RemoveAll(o.upper, name)
}

func (o *Overlay) Rename(oldName, newName string) error {
	return Rename(o.upper, oldName, newName)
}

func (o *Overlay) Symlink(target, name string) error {
	return Symlink(o.upper, target, name)
}

func (o *Overlay) Readlink(name string) (string, error) {
	name = Name(name)
	if target, err := Readlink(o.upper, name); err == nil {
		return target, nil
	} else if !isNotExist(err) {
		return "", err
	}
	return Readlink(o.lower, name)
}

func (o *Overlay) Chmod(name string, mode iofs.FileMode) error {
	return Chmod(o.upper, name, mode)
}

func (o *Overlay) Link(oldName, newName string) error {
	return Link(o.upper, oldName, newName)
}

func (o *Overlay) Chtimes(name string, atime, mtime time.Time) error {
	return Chtimes(o.upper, name, atime, mtime)
}

func isNotExist(err error) bool {
	return errors.Is(err, iofs.ErrNotExist)
}

var (
	_ iofs.FS         = (*Overlay)(nil)
	_ iofs.ReadFileFS = (*Overlay)(nil)
	_ iofs.ReadDirFS  = (*Overlay)(nil)
	_ iofs.StatFS     = (*Overlay)(nil)
)
