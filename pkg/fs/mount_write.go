package fs

import (
	iofs "io/fs"
	"path"
	"time"
)

func (m *Mountable) CreateFile(name string, data []byte, perm iofs.FileMode) error {
	filesystem, relative, _ := m.route(name)
	f, ok := filesystem.(CreateFileFS)
	if !ok {
		return ErrReadOnly
	}
	return f.CreateFile(relative, data, perm)
}

func (m *Mountable) WriteFile(name string, data []byte, perm iofs.FileMode) error {
	filesystem, relative, _ := m.route(name)
	f, ok := filesystem.(WriteFileFS)
	if !ok {
		return ErrReadOnly
	}
	return f.WriteFile(relative, data, perm)
}

func (m *Mountable) AppendFile(name string, data []byte, perm iofs.FileMode) error {
	filesystem, relative, _ := m.route(name)
	f, ok := filesystem.(AppendFileFS)
	if !ok {
		return ErrReadOnly
	}
	return f.AppendFile(relative, data, perm)
}

func (m *Mountable) Mkdir(name string, perm iofs.FileMode) error {
	name = Name(name)
	if m.IsMountPoint(name) {
		return iofs.ErrExist
	}
	filesystem, relative, _ := m.route(name)
	f, ok := filesystem.(MkdirFS)
	if !ok {
		return ErrReadOnly
	}
	return f.Mkdir(relative, perm)
}

func (m *Mountable) MkdirAll(name string, perm iofs.FileMode) error {
	name = Name(name)
	if m.IsMountPoint(name) || m.hasVirtualDir(name) {
		return nil
	}
	filesystem, relative, _ := m.route(name)
	if f, ok := filesystem.(MkdirAllFS); ok {
		return f.MkdirAll(relative, perm)
	}
	return ErrReadOnly
}

func (m *Mountable) Remove(name string) error {
	name = Name(name)
	if m.IsMountPoint(name) || m.hasVirtualDir(name) {
		return ErrBusy
	}
	filesystem, relative, _ := m.route(name)
	f, ok := filesystem.(RemoveFS)
	if !ok {
		return ErrReadOnly
	}
	return f.Remove(relative)
}

func (m *Mountable) RemoveAll(name string) error {
	name = Name(name)
	if m.IsMountPoint(name) || m.hasVirtualDir(name) {
		return ErrBusy
	}
	filesystem, relative, _ := m.route(name)
	f, ok := filesystem.(RemoveAllFS)
	if !ok {
		return ErrReadOnly
	}
	return f.RemoveAll(relative)
}

func (m *Mountable) Readlink(name string) (string, error) {
	filesystem, relative, _ := m.route(name)
	f, ok := filesystem.(ReadlinkFS)
	if !ok {
		return "", ErrReadOnly
	}
	return f.Readlink(relative)
}

func (m *Mountable) VirtualReadlink(name string) (string, error) {
	filesystem, relative, mount := m.route(name)
	if f, ok := filesystem.(ScopedVirtualReadlinkFS); ok {
		target, global, err := f.ScopedVirtualReadlink(relative)
		if err != nil || global || target == "" || target[0] != '/' || mount == "" {
			return target, err
		}
		return "/" + path.Join(mount, target[1:]), nil
	}
	if f, ok := filesystem.(VirtualReadlinkFS); ok {
		return f.VirtualReadlink(relative)
	}
	return m.Readlink(name)
}

func (m *Mountable) Symlink(target, name string) error {
	filesystem, relative, mount := m.route(name)
	if mount != "" && len(target) > 0 && target[0] == '/' {
		if f, ok := filesystem.(GlobalSymlinkFS); ok {
			return f.GlobalSymlink(target, relative)
		}
	}
	f, ok := filesystem.(SymlinkFS)
	if !ok {
		return ErrReadOnly
	}
	return f.Symlink(target, relative)
}

func (m *Mountable) Chmod(name string, mode iofs.FileMode) error {
	filesystem, relative, _ := m.route(name)
	f, ok := filesystem.(ChmodFS)
	if !ok {
		return ErrReadOnly
	}
	return f.Chmod(relative, mode)
}

func (m *Mountable) Chtimes(name string, atime, mtime time.Time) error {
	filesystem, relative, _ := m.route(name)
	f, ok := filesystem.(ChtimesFS)
	if !ok {
		return ErrReadOnly
	}
	return f.Chtimes(relative, atime, mtime)
}

func (m *Mountable) Link(oldName, newName string) error {
	oldFS, oldRelative, oldMount := m.route(oldName)
	_, newRelative, newMount := m.route(newName)
	if oldMount != newMount {
		return ErrCrossDevice
	}
	f, ok := oldFS.(LinkFS)
	if !ok {
		return ErrReadOnly
	}
	return f.Link(oldRelative, newRelative)
}

func (m *Mountable) Rename(oldName, newName string) error {
	oldName, newName = Name(oldName), Name(newName)
	if m.IsMountPoint(oldName) || m.hasVirtualDir(oldName) {
		return ErrBusy
	}
	oldFS, oldRelative, oldMount := m.route(oldName)
	_, newRelative, newMount := m.route(newName)
	if oldMount == newMount {
		f, ok := oldFS.(RenameFS)
		if !ok {
			return ErrReadOnly
		}
		return f.Rename(oldRelative, newRelative)
	}
	if err := m.copyAcross(oldName, newName); err != nil {
		return err
	}
	if info, err := m.Lstat(oldName); err == nil && info.IsDir() {
		return m.RemoveAll(oldName)
	}
	return m.Remove(oldName)
}

func (m *Mountable) copyAcross(src, dst string) error {
	info, err := m.Lstat(src)
	if err != nil {
		return err
	}
	if info.Mode()&iofs.ModeSymlink != 0 {
		target, err := m.Readlink(src)
		if err != nil {
			return err
		}
		return m.Symlink(target, dst)
	}
	if info.IsDir() {
		if err := m.MkdirAll(dst, info.Mode().Perm()); err != nil {
			return err
		}
		entries, err := m.ReadDir(src)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if err := m.copyAcross(path.Join(src, entry.Name()), path.Join(dst, entry.Name())); err != nil {
				return err
			}
		}
		return nil
	}
	data, err := m.ReadFile(src)
	if err != nil {
		return err
	}
	return m.WriteFile(dst, data, info.Mode().Perm())
}
