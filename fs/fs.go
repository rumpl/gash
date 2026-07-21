// Package fs defines io/fs-compatible capabilities and an in-memory implementation.
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
type RemoveFS interface{ Remove(name string) error }
type RemoveAllFS interface{ RemoveAll(name string) error }
type RenameFS interface {
	Rename(oldName, newName string) error
}
type SymlinkFS interface {
	Symlink(oldName, newName string) error
}
type ReadlinkFS interface {
	Readlink(name string) (string, error)
}
type LstatFS interface {
	Lstat(name string) (iofs.FileInfo, error)
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

// Memory is a concurrency-safe, quota-bounded io/fs implementation.
type Memory struct {
	mu          sync.RWMutex
	nodes       map[string]*node
	used, limit int64
}
type node struct {
	kind   nodeKind
	data   []byte
	target string
	mode   iofs.FileMode
	mtime  time.Time
}
type nodeKind uint8

const (
	regular nodeKind = iota
	directory
	symlink
)

func NewMemory(limit int64) *Memory {
	m := &Memory{nodes: map[string]*node{}, limit: limit}
	m.nodes["."] = &node{kind: directory, mode: iofs.ModeDir | 0755, mtime: time.Now()}
	return m
}
func valid(name string) error {
	if !iofs.ValidPath(name) {
		return &iofs.PathError{Op: "open", Path: name, Err: iofs.ErrInvalid}
	}
	return nil
}
func (m *Memory) resolveLocked(name string, followFinal bool) (string, *node, error) {
	if err := valid(name); err != nil {
		return "", nil, err
	}
	for links := 0; links <= 40; links++ {
		parts := strings.Split(name, "/")
		cur := "."
		restart := false
		for i, part := range parts {
			if part == "." || part == "" {
				continue
			}
			if cur == "." {
				cur = part
			} else {
				cur = path.Join(cur, part)
			}
			n := m.nodes[cur]
			if n == nil {
				return cur, nil, iofs.ErrNotExist
			}
			final := i == len(parts)-1
			if n.kind == symlink && (followFinal || !final) {
				rest := strings.Join(parts[i+1:], "/")
				target := n.target
				if strings.HasPrefix(target, "/") {
					target = Name(target)
				} else {
					target = path.Join(path.Dir(cur), target)
				}
				name = path.Clean(path.Join(target, rest))
				restart = true
				break
			}
			if !final && n.kind != directory {
				return cur, nil, ErrNotDir
			}
		}
		if !restart {
			return name, m.nodes[name], nil
		}
	}
	return "", nil, ErrLoop
}
func parent(name string) string {
	p := path.Dir(name)
	if p == "/" || p == "" {
		return "."
	}
	return p
}

func (m *Memory) Open(name string) (iofs.File, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	resolved, n, e := m.resolveLocked(name, true)
	if e != nil {
		return nil, &iofs.PathError{Op: "open", Path: name, Err: e}
	}
	f := &memFile{name: resolved, node: cloneNode(n)}
	if n.kind == directory {
		for p, v := range m.nodes {
			if p != resolved && parent(p) == resolved {
				f.entries = append(f.entries, entry{name: path.Base(p), node: cloneNode(v)})
			}
		}
		sort.Slice(f.entries, func(i, j int) bool { return f.entries[i].Name() < f.entries[j].Name() })
	}
	return f, nil
}
func (m *Memory) ReadFile(name string) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, n, e := m.resolveLocked(name, true)
	if e != nil {
		return nil, e
	}
	if n.kind == directory {
		return nil, ErrIsDir
	}
	return append([]byte(nil), n.data...), nil
}
func (m *Memory) ReadDir(name string) ([]iofs.DirEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	resolved, n, e := m.resolveLocked(name, true)
	if e != nil {
		return nil, e
	}
	if n.kind != directory {
		return nil, ErrNotDir
	}
	var out []iofs.DirEntry
	for p, v := range m.nodes {
		if p != resolved && parent(p) == resolved {
			out = append(out, entry{name: path.Base(p), node: cloneNode(v)})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out, nil
}
func (m *Memory) Stat(name string) (iofs.FileInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	resolved, n, e := m.resolveLocked(name, true)
	if e != nil {
		return nil, e
	}
	return fileInfo{name: path.Base(resolved), node: cloneNode(n)}, nil
}
func (m *Memory) Lstat(name string) (iofs.FileInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	resolved, n, e := m.resolveLocked(name, false)
	if e != nil {
		return nil, e
	}
	return fileInfo{name: path.Base(resolved), node: cloneNode(n)}, nil
}
func (m *Memory) WriteFile(name string, data []byte, perm iofs.FileMode) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := valid(name); err != nil {
		return err
	}
	p := m.nodes[parent(name)]
	if p == nil {
		return iofs.ErrNotExist
	}
	if p.kind != directory {
		return ErrNotDir
	}
	old := int64(0)
	if n := m.nodes[name]; n != nil {
		if n.kind == directory {
			return ErrIsDir
		}
		old = int64(len(n.data))
	}
	if m.limit > 0 && m.used-old+int64(len(data)) > m.limit {
		return ErrQuota
	}
	m.nodes[name] = &node{kind: regular, data: append([]byte(nil), data...), mode: perm.Perm(), mtime: time.Now()}
	m.used += int64(len(data)) - old
	return nil
}
func (m *Memory) AppendFile(name string, data []byte, perm iofs.FileMode) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := valid(name); err != nil {
		return err
	}
	n := m.nodes[name]
	if n == nil {
		p := m.nodes[parent(name)]
		if p == nil || p.kind != directory {
			return ErrNotDir
		}
		n = &node{kind: regular, mode: perm.Perm()}
		m.nodes[name] = n
	}
	if n.kind != regular {
		return ErrIsDir
	}
	if m.limit > 0 && m.used+int64(len(data)) > m.limit {
		return ErrQuota
	}
	n.data = append(n.data, data...)
	n.mtime = time.Now()
	m.used += int64(len(data))
	return nil
}
func (m *Memory) Mkdir(name string, perm iofs.FileMode) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := valid(name); err != nil {
		return err
	}
	if m.nodes[name] != nil {
		return iofs.ErrExist
	}
	p := m.nodes[parent(name)]
	if p == nil {
		return iofs.ErrNotExist
	}
	if p.kind != directory {
		return ErrNotDir
	}
	m.nodes[name] = &node{kind: directory, mode: iofs.ModeDir | perm.Perm(), mtime: time.Now()}
	return nil
}
func (m *Memory) MkdirAll(name string, perm iofs.FileMode) error {
	if err := valid(name); err != nil {
		return err
	}
	if name == "." {
		return nil
	}
	parts := strings.Split(name, "/")
	cur := ""
	for _, part := range parts {
		if cur == "" {
			cur = part
		} else {
			cur = path.Join(cur, part)
		}
		e := m.Mkdir(cur, perm)
		if e != nil && !errors.Is(e, iofs.ErrExist) {
			return e
		}
	}
	return nil
}
func (m *Memory) Remove(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if name == "." {
		return errors.New("cannot remove root")
	}
	n := m.nodes[name]
	if n == nil {
		return iofs.ErrNotExist
	}
	if n.kind == directory {
		for p := range m.nodes {
			if parent(p) == name {
				return ErrNotEmpty
			}
		}
	}
	m.used -= int64(len(n.data))
	delete(m.nodes, name)
	return nil
}
func (m *Memory) RemoveAll(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if name == "." {
		return errors.New("cannot remove root")
	}
	prefix := name + "/"
	for p, n := range m.nodes {
		if p == name || strings.HasPrefix(p, prefix) {
			m.used -= int64(len(n.data))
			delete(m.nodes, p)
		}
	}
	return nil
}
func (m *Memory) Rename(oldName, newName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := m.nodes[oldName]
	if n == nil {
		return iofs.ErrNotExist
	}
	if strings.HasPrefix(newName+"/", oldName+"/") {
		return errors.New("cannot move directory into itself")
	}
	p := m.nodes[parent(newName)]
	if p == nil || p.kind != directory {
		return ErrNotDir
	}
	if existing := m.nodes[newName]; existing != nil {
		if existing.kind == directory {
			for q := range m.nodes {
				if parent(q) == newName {
					return ErrNotEmpty
				}
			}
		}
		m.used -= int64(len(existing.data))
		delete(m.nodes, newName)
	}
	moving := map[string]*node{}
	for p, v := range m.nodes {
		if p == oldName || strings.HasPrefix(p, oldName+"/") {
			moving[p] = v
			delete(m.nodes, p)
		}
	}
	for p, v := range moving {
		m.nodes[newName+strings.TrimPrefix(p, oldName)] = v
	}
	return nil
}
func (m *Memory) Symlink(target, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := valid(name); err != nil {
		return err
	}
	if m.nodes[name] != nil {
		return iofs.ErrExist
	}
	p := m.nodes[parent(name)]
	if p == nil || p.kind != directory {
		return ErrNotDir
	}
	m.nodes[name] = &node{kind: symlink, target: target, mode: iofs.ModeSymlink | 0777, mtime: time.Now()}
	return nil
}
func (m *Memory) Readlink(name string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, n, e := m.resolveLocked(name, false)
	if e != nil {
		return "", e
	}
	if n.kind != symlink {
		return "", errors.New("not a symbolic link")
	}
	return n.target, nil
}
func (m *Memory) Used() int64 { m.mu.RLock(); defer m.mu.RUnlock(); return m.used }

func cloneNode(n *node) *node { c := *n; c.data = append([]byte(nil), n.data...); return &c }

type fileInfo struct {
	name string
	node *node
}

func (i fileInfo) Name() string        { return i.name }
func (i fileInfo) Size() int64         { return int64(len(i.node.data)) }
func (i fileInfo) Mode() iofs.FileMode { return i.node.mode }
func (i fileInfo) ModTime() time.Time  { return i.node.mtime }
func (i fileInfo) IsDir() bool         { return i.node.kind == directory }
func (i fileInfo) Sys() any            { return nil }

type entry struct {
	name string
	node *node
}

func (e entry) Name() string                 { return e.name }
func (e entry) IsDir() bool                  { return e.node.kind == directory }
func (e entry) Type() iofs.FileMode          { return e.node.mode.Type() }
func (e entry) Info() (iofs.FileInfo, error) { return fileInfo{name: e.name, node: e.node}, nil }

type memFile struct {
	name      string
	node      *node
	offset    int
	dirOffset int
	entries   []iofs.DirEntry
}

func (f *memFile) Stat() (iofs.FileInfo, error) {
	return fileInfo{name: path.Base(f.name), node: f.node}, nil
}
func (f *memFile) Close() error { return nil }
func (f *memFile) Read(p []byte) (int, error) {
	if f.node.kind == directory {
		return 0, ErrIsDir
	}
	if f.offset >= len(f.node.data) {
		return 0, io.EOF
	}
	n := copy(p, f.node.data[f.offset:])
	f.offset += n
	return n, nil
}
func (f *memFile) ReadDir(n int) ([]iofs.DirEntry, error) {
	if f.node.kind != directory {
		return nil, ErrNotDir
	}
	if f.dirOffset >= len(f.entries) {
		if n > 0 {
			return nil, io.EOF
		}
		return []iofs.DirEntry{}, nil
	}
	end := len(f.entries)
	if n > 0 && f.dirOffset+n < end {
		end = f.dirOffset + n
	}
	out := append([]iofs.DirEntry(nil), f.entries[f.dirOffset:end]...)
	f.dirOffset = end
	return out, nil
}

var _ iofs.FS = (*Memory)(nil)
var _ iofs.ReadFileFS = (*Memory)(nil)
var _ iofs.ReadDirFS = (*Memory)(nil)
var _ iofs.StatFS = (*Memory)(nil)
