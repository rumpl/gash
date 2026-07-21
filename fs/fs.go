// Package fs provides the sandboxed filesystem used by gash.
package fs

import (
	"errors"
	"io/fs"
	"path"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	ErrNotExist = fs.ErrNotExist
	ErrExist    = fs.ErrExist
	ErrNotDir   = errors.New("not a directory")
	ErrIsDir    = errors.New("is a directory")
	ErrNotEmpty = errors.New("directory not empty")
	ErrQuota    = errors.New("filesystem quota exceeded")
	ErrLoop     = errors.New("too many symbolic links")
)

type Kind uint8

const (
	File Kind = iota
	Directory
	Symlink
)

type Info struct {
	Path    string
	Kind    Kind
	Mode    fs.FileMode
	Size    int64
	ModTime time.Time
	Target  string
}

type FileSystem interface {
	ReadFile(name string) ([]byte, error)
	WriteFile(name string, data []byte, mode fs.FileMode) error
	AppendFile(name string, data []byte) error
	Mkdir(name string, mode fs.FileMode, recursive bool) error
	ReadDir(name string) ([]Info, error)
	Stat(name string) (Info, error)
	Lstat(name string) (Info, error)
	Remove(name string, recursive bool) error
	Rename(oldName, newName string) error
	Symlink(target, name string) error
	Readlink(name string) (string, error)
	Resolve(base, name string) string
}

type node struct {
	kind   Kind
	data   []byte
	target string
	mode   fs.FileMode
	mtime  time.Time
}

// Memory is a concurrency-safe, byte-bounded Unix-like in-memory filesystem.
type Memory struct {
	mu          sync.RWMutex
	nodes       map[string]*node
	used, limit int64
}

func NewMemory(limit int64) *Memory {
	m := &Memory{nodes: make(map[string]*node), limit: limit}
	m.nodes["/"] = &node{kind: Directory, mode: 0755 | fs.ModeDir, mtime: time.Now()}
	return m
}

func (m *Memory) Resolve(base, name string) string {
	if name == "" {
		return path.Clean(base)
	}
	if strings.HasPrefix(name, "/") {
		return path.Clean(name)
	}
	return path.Clean(path.Join(base, name))
}

func clean(name string) string {
	if name == "" {
		return "/"
	}
	if name[0] != '/' {
		name = "/" + name
	}
	return path.Clean(name)
}

func (m *Memory) resolveLocked(name string, followFinal bool) (string, *node, error) {
	name = clean(name)
	for links := 0; links <= 40; links++ {
		parts := strings.Split(strings.TrimPrefix(name, "/"), "/")
		cur := "/"
		restarted := false
		for i, part := range parts {
			if part == "" {
				continue
			}
			cur = path.Join(cur, part)
			n, ok := m.nodes[cur]
			if !ok {
				return cur, nil, ErrNotExist
			}
			final := i == len(parts)-1
			if n.kind == Symlink && (followFinal || !final) {
				rest := strings.Join(parts[i+1:], "/")
				base := n.target
				if !strings.HasPrefix(base, "/") {
					base = path.Join(path.Dir(cur), base)
				}
				name = path.Join(base, rest)
				restarted = true
				break
			}
			if !final && n.kind != Directory {
				return cur, nil, ErrNotDir
			}
		}
		if !restarted {
			return name, m.nodes[name], nil
		}
	}
	return "", nil, ErrLoop
}

func (m *Memory) ReadFile(name string) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, n, err := m.resolveLocked(name, true)
	if err != nil {
		return nil, err
	}
	if n.kind == Directory {
		return nil, ErrIsDir
	}
	if n.kind != File {
		return nil, ErrNotExist
	}
	return append([]byte(nil), n.data...), nil
}

func (m *Memory) WriteFile(name string, data []byte, mode fs.FileMode) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	name = clean(name)
	if _, p, err := m.resolveLocked(path.Dir(name), true); err != nil || p.kind != Directory {
		if err != nil {
			return err
		}
		return ErrNotDir
	}
	old := int64(0)
	if n := m.nodes[name]; n != nil {
		if n.kind == Directory {
			return ErrIsDir
		}
		old = int64(len(n.data))
	}
	if m.limit > 0 && m.used-old+int64(len(data)) > m.limit {
		return ErrQuota
	}
	m.nodes[name] = &node{kind: File, data: append([]byte(nil), data...), mode: mode.Perm(), mtime: time.Now()}
	m.used += int64(len(data)) - old
	return nil
}

func (m *Memory) AppendFile(name string, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	name = clean(name)
	n := m.nodes[name]
	if n == nil {
		if p := m.nodes[path.Dir(name)]; p == nil || p.kind != Directory {
			return ErrNotDir
		}
		n = &node{kind: File, mode: 0644}
		m.nodes[name] = n
	}
	if n.kind != File {
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

func (m *Memory) Mkdir(name string, mode fs.FileMode, recursive bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	name = clean(name)
	if n := m.nodes[name]; n != nil {
		if recursive && n.kind == Directory {
			return nil
		}
		return ErrExist
	}
	parts := strings.Split(strings.TrimPrefix(name, "/"), "/")
	cur := "/"
	for i, part := range parts {
		cur = path.Join(cur, part)
		n := m.nodes[cur]
		if n != nil {
			if n.kind != Directory {
				return ErrNotDir
			}
			continue
		}
		if !recursive && i != len(parts)-1 {
			return ErrNotExist
		}
		m.nodes[cur] = &node{kind: Directory, mode: mode.Perm() | fs.ModeDir, mtime: time.Now()}
	}
	return nil
}

func info(name string, n *node) Info {
	return Info{Path: name, Kind: n.kind, Mode: n.mode, Size: int64(len(n.data)), ModTime: n.mtime, Target: n.target}
}
func (m *Memory) stat(name string, follow bool) (Info, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p, n, e := m.resolveLocked(name, follow)
	if e != nil {
		return Info{}, e
	}
	return info(p, n), nil
}
func (m *Memory) Stat(name string) (Info, error)  { return m.stat(name, true) }
func (m *Memory) Lstat(name string) (Info, error) { return m.stat(name, false) }
func (m *Memory) ReadDir(name string) ([]Info, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p, n, e := m.resolveLocked(name, true)
	if e != nil {
		return nil, e
	}
	if n.kind != Directory {
		return nil, ErrNotDir
	}
	var out []Info
	for q, v := range m.nodes {
		if q != p && path.Dir(q) == p {
			out = append(out, info(q, v))
		}
	}
	sort.Slice(out, func(i, j int) bool { return path.Base(out[i].Path) < path.Base(out[j].Path) })
	return out, nil
}
func (m *Memory) Remove(name string, recursive bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	name = clean(name)
	if name == "/" {
		return errors.New("cannot remove root")
	}
	n := m.nodes[name]
	if n == nil {
		return ErrNotExist
	}
	prefix := name + "/"
	if n.kind == Directory {
		for p := range m.nodes {
			if strings.HasPrefix(p, prefix) && !recursive {
				return ErrNotEmpty
			}
		}
	}
	for p, v := range m.nodes {
		if p == name || strings.HasPrefix(p, prefix) {
			m.used -= int64(len(v.data))
			delete(m.nodes, p)
		}
	}
	return nil
}
func (m *Memory) Rename(oldName, newName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	oldName, newName = clean(oldName), clean(newName)
	n := m.nodes[oldName]
	if n == nil {
		return ErrNotExist
	}
	if p := m.nodes[path.Dir(newName)]; p == nil || p.kind != Directory {
		return ErrNotDir
	}
	if existing := m.nodes[newName]; existing != nil {
		if existing.kind == Directory {
			for p := range m.nodes {
				if strings.HasPrefix(p, newName+"/") {
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
	name = clean(name)
	if m.nodes[name] != nil {
		return ErrExist
	}
	if p := m.nodes[path.Dir(name)]; p == nil || p.kind != Directory {
		return ErrNotDir
	}
	m.nodes[name] = &node{kind: Symlink, target: target, mode: fs.ModeSymlink | 0777, mtime: time.Now()}
	return nil
}
func (m *Memory) Readlink(name string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, n, e := m.resolveLocked(name, false)
	if e != nil {
		return "", e
	}
	if n.kind != Symlink {
		return "", errors.New("not a symbolic link")
	}
	return n.target, nil
}
func (m *Memory) Used() int64 { m.mu.RLock(); defer m.mu.RUnlock(); return m.used }
