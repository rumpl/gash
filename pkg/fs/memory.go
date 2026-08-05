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
	links  uint64
}
type nodeKind uint8

const (
	regular nodeKind = iota
	directory
	symlink
)

func NewMemory(limit int64) *Memory {
	m := &Memory{nodes: map[string]*node{}, limit: limit}
	m.nodes["."] = &node{kind: directory, mode: iofs.ModeDir | 0o755, mtime: time.Now(), links: 1}
	return m
}

func valid(name string) error {
	if name == "" || strings.HasPrefix(name, "/") || !iofs.ValidPath(name) {
		return &iofs.PathError{Op: "open", Path: name, Err: iofs.ErrInvalid}
	}
	return nil
}

func (m *Memory) resolveLocked(name string, followFinal bool) (string, *node, error) {
	for links := 0; links <= 40; links++ {
		if err := valid(name); err != nil {
			return "", nil, err
		}
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

func (m *Memory) CreateFile(name string, data []byte, perm iofs.FileMode) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := valid(name); err != nil {
		return err
	}
	if _, _, err := m.resolveLocked(name, false); err == nil {
		return iofs.ErrExist
	} else if !errors.Is(err, iofs.ErrNotExist) {
		return err
	}
	resolvedParent, p, err := m.resolveLocked(parent(name), true)
	if err != nil {
		return err
	}
	if p.kind != directory {
		return ErrNotDir
	}
	if m.limit > 0 && m.used+int64(len(data)) > m.limit {
		return ErrQuota
	}
	resolved := path.Join(resolvedParent, path.Base(name))
	if m.nodes[resolved] != nil {
		return iofs.ErrExist
	}
	m.nodes[resolved] = &node{kind: regular, data: append([]byte(nil), data...), mode: perm.Perm(), mtime: time.Now(), links: 1}
	m.used += int64(len(data))
	return nil
}

func (m *Memory) WriteFile(name string, data []byte, perm iofs.FileMode) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := valid(name); err != nil {
		return err
	}
	resolved, existingNode, e := m.resolveLocked(name, true)
	if e != nil && !errors.Is(e, iofs.ErrNotExist) {
		return e
	}
	if existingNode == nil {
		resolvedParent, p, e := m.resolveLocked(parent(name), true)
		if e != nil {
			return e
		}
		if p.kind != directory {
			return ErrNotDir
		}
		resolved = path.Join(resolvedParent, path.Base(name))
	}
	old := int64(0)
	if existingNode != nil {
		if existingNode.kind == directory {
			return ErrIsDir
		}
		old = int64(len(existingNode.data))
	}
	if m.limit > 0 && m.used-old+int64(len(data)) > m.limit {
		return ErrQuota
	}
	if existingNode != nil {
		m.used += int64(len(data)) - old
		existingNode.data = append([]byte(nil), data...)
		existingNode.mode = existingNode.mode.Type() | perm.Perm()
		existingNode.mtime = time.Now()
	} else {
		m.nodes[resolved] = &node{kind: regular, data: append([]byte(nil), data...), mode: perm.Perm(), mtime: time.Now(), links: 1}
		m.used += int64(len(data))
	}
	return nil
}

func (m *Memory) AppendFile(name string, data []byte, perm iofs.FileMode) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := valid(name); err != nil {
		return err
	}
	resolved, n, e := m.resolveLocked(name, true)
	if e != nil && !errors.Is(e, iofs.ErrNotExist) {
		return e
	}
	if n == nil {
		resolvedParent, p, parentErr := m.resolveLocked(parent(name), true)
		if parentErr != nil {
			return parentErr
		}
		if p.kind != directory {
			return ErrNotDir
		}
		resolved = path.Join(resolvedParent, path.Base(name))
		n = &node{kind: regular, mode: perm.Perm(), links: 1, mtime: time.Now()}
		m.nodes[resolved] = n
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
	if _, _, err := m.resolveLocked(name, true); err == nil {
		return iofs.ErrExist
	} else if !errors.Is(err, iofs.ErrNotExist) {
		return err
	}
	resolvedParent, p, err := m.resolveLocked(parent(name), true)
	if err != nil {
		return err
	}
	if p.kind != directory {
		return ErrNotDir
	}
	resolved := path.Join(resolvedParent, path.Base(name))
	if m.nodes[resolved] != nil {
		return iofs.ErrExist
	}
	m.nodes[resolved] = &node{kind: directory, mode: iofs.ModeDir | perm.Perm(), mtime: time.Now(), links: 1}
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
	if err := valid(name); err != nil {
		return err
	}
	resolved, n, e := m.resolveLocked(name, false)
	if e != nil {
		return e
	}
	if n.kind == directory {
		for p := range m.nodes {
			if parent(p) == resolved {
				return ErrNotEmpty
			}
		}
	}
	if n.links > 0 {
		n.links--
	}
	if n.links == 0 {
		m.used -= int64(len(n.data))
	}
	delete(m.nodes, resolved)
	return nil
}

func (m *Memory) RemoveAll(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if name == "." {
		return errors.New("cannot remove root")
	}
	if err := valid(name); err != nil {
		return err
	}
	resolved, _, e := m.resolveLocked(name, false)
	if e != nil {
		if errors.Is(e, iofs.ErrNotExist) {
			return nil
		}
		return e
	}
	prefix := resolved + "/"
	for p, n := range m.nodes {
		if p == resolved || strings.HasPrefix(p, prefix) {
			if n.links > 0 {
				n.links--
			}
			if n.links == 0 {
				m.used -= int64(len(n.data))
			}
			delete(m.nodes, p)
		}
	}
	return nil
}

func (m *Memory) Rename(oldName, newName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := valid(oldName); err != nil {
		return err
	}
	if err := valid(newName); err != nil {
		return err
	}
	oldResolved, n, e := m.resolveLocked(oldName, false)
	if e != nil {
		return e
	}
	newParentResolved, p, e := m.resolveLocked(parent(newName), true)
	if e != nil {
		return e
	}
	if p.kind != directory {
		return ErrNotDir
	}
	newResolved := path.Join(newParentResolved, path.Base(newName))
	if strings.HasPrefix(newResolved+"/", oldResolved+"/") {
		return errors.New("cannot move directory into itself")
	}
	if existing := m.nodes[newResolved]; existing != nil {
		if existing.kind == directory && n.kind != directory {
			return ErrIsDir
		}
		if existing.kind != directory && n.kind == directory {
			return ErrNotDir
		}
		if existing.kind == directory {
			for q := range m.nodes {
				if parent(q) == newResolved {
					return ErrNotEmpty
				}
			}
		}
		if existing.links > 0 {
			existing.links--
		}
		if existing.links == 0 {
			m.used -= int64(len(existing.data))
		}
		delete(m.nodes, newResolved)
	}
	moving := map[string]*node{}
	for p, v := range m.nodes {
		if p == oldResolved || strings.HasPrefix(p, oldResolved+"/") {
			moving[p] = v
			delete(m.nodes, p)
		}
	}
	for p, v := range moving {
		m.nodes[newResolved+strings.TrimPrefix(p, oldResolved)] = v
	}
	return nil
}

func (m *Memory) Symlink(target, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := valid(name); err != nil {
		return err
	}
	if _, _, err := m.resolveLocked(name, false); err == nil {
		return iofs.ErrExist
	} else if !errors.Is(err, iofs.ErrNotExist) {
		return err
	}
	resolvedParent, p, err := m.resolveLocked(parent(name), true)
	if err != nil {
		return err
	}
	if p.kind != directory {
		return ErrNotDir
	}
	resolved := path.Join(resolvedParent, path.Base(name))
	m.nodes[resolved] = &node{kind: symlink, target: target, mode: iofs.ModeSymlink | 0o777, mtime: time.Now(), links: 1}
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

func (m *Memory) Link(oldName, newName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := valid(newName); err != nil {
		return err
	}
	_, n, err := m.resolveLocked(oldName, true)
	if err != nil {
		return err
	}
	if n.kind != regular {
		return errors.New("hard link source is not a regular file")
	}
	if m.nodes[newName] != nil {
		return iofs.ErrExist
	}
	p := m.nodes[parent(newName)]
	if p == nil {
		return iofs.ErrNotExist
	}
	if p.kind != directory {
		return ErrNotDir
	}
	n.links++
	m.nodes[newName] = n
	return nil
}

func (m *Memory) Chmod(name string, mode iofs.FileMode) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, n, err := m.resolveLocked(name, true)
	if err != nil {
		return err
	}
	n.mode = n.mode.Type() | mode.Perm()
	n.mtime = time.Now()
	return nil
}

func (m *Memory) Chtimes(name string, _ time.Time, mtime time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, n, err := m.resolveLocked(name, true)
	if err != nil {
		return err
	}
	n.mtime = mtime
	return nil
}

func (m *Memory) Used() int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.used
}

func cloneNode(n *node) *node {
	c := *n
	c.data = append([]byte(nil), n.data...)
	return &c
}

type fileInfo struct {
	name string
	node *node
}

func (i fileInfo) Name() string {
	return i.name
}

func (i fileInfo) Size() int64 {
	return int64(len(i.node.data))
}

func (i fileInfo) Mode() iofs.FileMode {
	return i.node.mode
}

func (i fileInfo) ModTime() time.Time {
	return i.node.mtime
}

func (i fileInfo) IsDir() bool {
	return i.node.kind == directory
}

func (i fileInfo) Sys() any {
	return nil
}

type entry struct {
	name string
	node *node
}

func (e entry) Name() string {
	return e.name
}

func (e entry) IsDir() bool {
	return e.node.kind == directory
}

func (e entry) Type() iofs.FileMode {
	return e.node.mode.Type()
}

func (e entry) Info() (iofs.FileInfo, error) {
	return fileInfo{name: e.name, node: e.node}, nil
}

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

func (f *memFile) Close() error {
	return nil
}

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

var (
	_ iofs.FS         = (*Memory)(nil)
	_ iofs.ReadFileFS = (*Memory)(nil)
	_ iofs.ReadDirFS  = (*Memory)(nil)
	_ iofs.StatFS     = (*Memory)(nil)
)
