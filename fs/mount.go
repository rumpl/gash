package fs

import (
	"errors"
	"io"
	iofs "io/fs"
	"path"
	"sort"
	"strings"
	"sync"
	"time"
)

var ErrBusy = errors.New("resource busy")

type MountConfig struct {
	Point string
	FS    iofs.FS
}
type MountableOptions struct {
	Base   iofs.FS
	Mounts []MountConfig
}
type mountEntry struct {
	point string
	fs    iofs.FS
}

// Mountable combines independent io/fs implementations into one namespace.
// Mount paths use io/fs form ("mnt/data"); a leading slash is also accepted.
type Mountable struct {
	mu     sync.RWMutex
	base   iofs.FS
	mounts map[string]mountEntry
}

func NewMountable(options MountableOptions) (*Mountable, error) {
	base := options.Base
	if base == nil {
		base = NewMemory(0)
	}
	m := &Mountable{base: base, mounts: map[string]mountEntry{}}
	for _, cfg := range options.Mounts {
		if err := m.Mount(cfg.Point, cfg.FS); err != nil {
			return nil, err
		}
	}
	return m, nil
}
func (m *Mountable) Mount(point string, filesystem iofs.FS) error {
	if filesystem == nil {
		return errors.New("mount filesystem is nil")
	}
	raw := strings.TrimPrefix(point, "/")
	for _, part := range strings.Split(raw, "/") {
		if part == "." || part == ".." {
			return errors.New("mount point contains . or .. segment")
		}
	}
	point = Name(point)
	if point == "." {
		return errors.New("cannot mount at root")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for existing := range m.mounts {
		if existing == point {
			continue
		}
		if strings.HasPrefix(point, existing+"/") || strings.HasPrefix(existing, point+"/") {
			return errors.New("nested mount points are not allowed")
		}
	}
	m.mounts[point] = mountEntry{point: point, fs: filesystem}
	return nil
}
func (m *Mountable) Unmount(point string) error {
	point = Name(point)
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.mounts[point]; !ok {
		return iofs.ErrNotExist
	}
	delete(m.mounts, point)
	return nil
}
func (m *Mountable) Mounts() []MountConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]MountConfig, 0, len(m.mounts))
	for _, entry := range m.mounts {
		out = append(out, MountConfig{Point: entry.point, FS: entry.fs})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Point < out[j].Point })
	return out
}
func (m *Mountable) IsMountPoint(name string) bool {
	name = Name(name)
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.mounts[name]
	return ok
}
func (m *Mountable) route(name string) (iofs.FS, string, string) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	best := ""
	var filesystem iofs.FS
	for point, entry := range m.mounts {
		if name == point || strings.HasPrefix(name, point+"/") {
			if len(point) > len(best) {
				best = point
				filesystem = entry.fs
			}
		}
	}
	if filesystem == nil {
		return m.base, name, ""
	}
	relative := strings.TrimPrefix(name, best)
	relative = strings.TrimPrefix(relative, "/")
	if relative == "" {
		relative = "."
	}
	return filesystem, relative, best
}
func (m *Mountable) childMounts(name string) []string {
	prefix := ""
	if name != "." {
		prefix = name + "/"
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	seen := map[string]bool{}
	for point := range m.mounts {
		if strings.HasPrefix(point, prefix) {
			rest := strings.TrimPrefix(point, prefix)
			if child := strings.Split(rest, "/")[0]; child != "" {
				seen[child] = true
			}
		}
	}
	out := make([]string, 0, len(seen))
	for child := range seen {
		out = append(out, child)
	}
	sort.Strings(out)
	return out
}
func (m *Mountable) hasVirtualDir(name string) bool { return len(m.childMounts(name)) > 0 }

func (m *Mountable) Open(name string) (iofs.File, error) {
	if err := valid(name); err != nil {
		return nil, err
	}
	if !m.hasVirtualDir(name) {
		filesystem, relative, mount := m.route(name)
		file, err := filesystem.Open(relative)
		if err != nil {
			return nil, err
		}
		if mount != "" && name == mount {
			return &namedFile{File: file, name: path.Base(name)}, nil
		}
		return file, nil
	}
	entries, e := m.ReadDir(name)
	if e != nil {
		return nil, e
	}
	return &mountDirFile{name: path.Base(Name(name)), entries: entries}, nil
}
func (m *Mountable) ReadFile(name string) ([]byte, error) {
	if err := valid(name); err != nil {
		return nil, err
	}
	filesystem, relative, _ := m.route(name)
	return iofs.ReadFile(filesystem, relative)
}
func (m *Mountable) Stat(name string) (iofs.FileInfo, error) {
	if err := valid(name); err != nil {
		return nil, err
	}
	filesystem, relative, mount := m.route(name)
	info, err := iofs.Stat(filesystem, relative)
	if err == nil {
		if mount != "" && name == mount {
			return renamedInfo{name: path.Base(name), FileInfo: info}, nil
		}
		return info, nil
	}
	if mount != "" && name == mount {
		return virtualDirInfo(path.Base(name)), nil
	}
	if m.hasVirtualDir(name) {
		return virtualDirInfo(path.Base(name)), nil
	}
	return nil, err
}
func (m *Mountable) Lstat(name string) (iofs.FileInfo, error) {
	if err := valid(name); err != nil {
		return nil, err
	}
	filesystem, relative, mount := m.route(name)
	var info iofs.FileInfo
	var err error
	if f, ok := filesystem.(LstatFS); ok {
		info, err = f.Lstat(relative)
	} else {
		info, err = iofs.Stat(filesystem, relative)
	}
	if err == nil {
		if mount != "" && name == mount {
			return renamedInfo{name: path.Base(name), FileInfo: info}, nil
		}
		return info, nil
	}
	if mount != "" && name == mount {
		return virtualDirInfo(path.Base(name)), nil
	}
	if m.hasVirtualDir(name) {
		return virtualDirInfo(path.Base(name)), nil
	}
	return nil, err
}
func (m *Mountable) ReadDir(name string) ([]iofs.DirEntry, error) {
	if err := valid(name); err != nil {
		return nil, err
	}
	filesystem, relative, _ := m.route(name)
	entries, err := iofs.ReadDir(filesystem, relative)
	children := m.childMounts(name)
	if err != nil && len(children) == 0 {
		return nil, err
	}
	byName := map[string]iofs.DirEntry{}
	for _, entry := range entries {
		byName[entry.Name()] = entry
	}
	for _, child := range children {
		childPath := child
		if name != "." {
			childPath = path.Join(name, child)
		}
		childFS, childRelative, _ := m.route(childPath)
		info, statErr := iofs.Stat(childFS, childRelative)
		if statErr != nil {
			info = virtualDirInfo(child)
		} else {
			info = renamedInfo{name: child, FileInfo: info}
		}
		byName[child] = mountedEntry{name: child, info: info}
	}
	out := make([]iofs.DirEntry, 0, len(byName))
	for _, entry := range byName {
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out, nil
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
func (m *Mountable) Symlink(target, name string) error {
	filesystem, relative, _ := m.route(name)
	f, ok := filesystem.(SymlinkFS)
	if !ok {
		return ErrReadOnly
	}
	return f.Symlink(target, relative)
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

type virtualDirInfo string

func (i virtualDirInfo) Name() string      { return string(i) }
func (virtualDirInfo) Size() int64         { return 0 }
func (virtualDirInfo) Mode() iofs.FileMode { return iofs.ModeDir | 0755 }
func (virtualDirInfo) ModTime() time.Time  { return time.Time{} }
func (virtualDirInfo) IsDir() bool         { return true }
func (virtualDirInfo) Sys() any            { return nil }

type renamedInfo struct {
	name string
	iofs.FileInfo
}

func (i renamedInfo) Name() string { return i.name }

type mountedEntry struct {
	name string
	info iofs.FileInfo
}

func (e mountedEntry) Name() string                 { return e.name }
func (e mountedEntry) IsDir() bool                  { return e.info.IsDir() }
func (e mountedEntry) Type() iofs.FileMode          { return e.info.Mode().Type() }
func (e mountedEntry) Info() (iofs.FileInfo, error) { return e.info, nil }

type namedFile struct {
	iofs.File
	name string
}

func (f *namedFile) Stat() (iofs.FileInfo, error) {
	info, err := f.File.Stat()
	if err != nil {
		return nil, err
	}
	return renamedInfo{name: f.name, FileInfo: info}, nil
}
func (f *namedFile) ReadDir(n int) ([]iofs.DirEntry, error) {
	dir, ok := f.File.(iofs.ReadDirFile)
	if !ok {
		return nil, ErrNotDir
	}
	return dir.ReadDir(n)
}

type mountDirFile struct {
	name    string
	entries []iofs.DirEntry
	offset  int
}

func (f *mountDirFile) Stat() (iofs.FileInfo, error) { return virtualDirInfo(f.name), nil }
func (*mountDirFile) Close() error                   { return nil }
func (*mountDirFile) Read([]byte) (int, error)       { return 0, ErrIsDir }
func (f *mountDirFile) ReadDir(n int) ([]iofs.DirEntry, error) {
	if f.offset >= len(f.entries) {
		if n > 0 {
			return nil, io.EOF
		}
		return []iofs.DirEntry{}, nil
	}
	end := len(f.entries)
	if n > 0 && f.offset+n < end {
		end = f.offset + n
	}
	out := append([]iofs.DirEntry(nil), f.entries[f.offset:end]...)
	f.offset = end
	return out, nil
}

var _ iofs.FS = (*Mountable)(nil)
var _ iofs.ReadFileFS = (*Mountable)(nil)
var _ iofs.ReadDirFS = (*Mountable)(nil)
var _ iofs.StatFS = (*Mountable)(nil)
