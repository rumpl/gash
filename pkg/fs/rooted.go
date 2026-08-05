package fs

import (
	"encoding/base64"
	"errors"
	iofs "io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// Rooted exposes a host directory through io/fs while rejecting lexical path
// traversal and symlink escapes. Write operations resolve every parent through
// the root before touching the host filesystem.
type Rooted struct {
	root string
}

const rootedGlobalSymlinkPrefix = ".gash-global-symlink."

func NewRooted(root string) (*Rooted, error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	real, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(real)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, ErrNotDir
	}
	return &Rooted{root: real}, nil
}

func (r *Rooted) Open(name string) (iofs.File, error) {
	host, err := r.hostPath(name, true)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(host)
	if err != nil {
		return nil, rootedPathError("open", name, err)
	}
	return file, nil
}

func (r *Rooted) ReadFile(name string) ([]byte, error) {
	host, err := r.hostPath(name, true)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(host)
	if err != nil {
		return nil, rootedPathError("read", name, err)
	}
	return data, nil
}

func (r *Rooted) ReadDir(name string) ([]iofs.DirEntry, error) {
	host, err := r.hostPath(name, true)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(host)
	if err != nil {
		return nil, rootedPathError("readdir", name, err)
	}
	return entries, nil
}

func (r *Rooted) Stat(name string) (iofs.FileInfo, error) {
	host, err := r.hostPath(name, true)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(host)
	if err != nil {
		return nil, rootedPathError("stat", name, err)
	}
	return info, nil
}

func (r *Rooted) Lstat(name string) (iofs.FileInfo, error) {
	host, err := r.lexicalHostPath(name)
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(host)
	if err != nil {
		return nil, rootedPathError("lstat", name, err)
	}
	return info, nil
}

func (r *Rooted) Readlink(name string) (string, error) {
	target, _, err := r.ScopedVirtualReadlink(name)
	return target, err
}

// VirtualReadlink returns Rooted link targets in its virtual namespace.
func (r *Rooted) VirtualReadlink(name string) (string, error) {
	target, _, err := r.ScopedVirtualReadlink(name)
	return target, err
}

func (r *Rooted) ScopedVirtualReadlink(name string) (string, bool, error) {
	host, err := r.lexicalHostPath(name)
	if err != nil {
		return "", false, err
	}
	target, err := os.Readlink(host)
	if err != nil {
		return "", false, rootedPathError("readlink", name, err)
	}
	if strings.HasPrefix(target, rootedGlobalSymlinkPrefix) {
		encoded := strings.TrimPrefix(target, rootedGlobalSymlinkPrefix)
		decoded, decodeErr := base64.RawURLEncoding.DecodeString(encoded)
		if decodeErr != nil || len(decoded) == 0 || decoded[0] != '/' {
			return "", false, iofs.ErrPermission
		}
		return string(decoded), true, nil
	}
	if !filepath.IsAbs(target) {
		return target, false, nil
	}
	clean := filepath.Clean(target)
	if resolved, resolveErr := filepath.EvalSymlinks(clean); resolveErr == nil {
		clean = resolved
	}
	if !r.inside(clean) {
		// Unmarked absolute targets outside the backing root are host escape
		// links. Never expose their target, including to missing-path walkers.
		return "", false, iofs.ErrPermission
	}
	relative, err := filepath.Rel(r.root, clean)
	if err != nil {
		return "", false, iofs.ErrPermission
	}
	if relative == "." {
		return "/", false, nil
	}
	return "/" + filepath.ToSlash(relative), false, nil
}

func (r *Rooted) CreateFile(name string, data []byte, perm iofs.FileMode) error {
	host, err := r.hostCreatePath(name)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(host, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		return rootedPathError("create", name, err)
	}
	if _, err = file.Write(data); err != nil {
		_ = file.Close()
		_ = os.Remove(host)
		return rootedPathError("create", name, err)
	}
	if err = file.Close(); err != nil {
		_ = os.Remove(host)
		return rootedPathError("create", name, err)
	}
	return nil
}

func (r *Rooted) WriteFile(name string, data []byte, perm iofs.FileMode) error {
	host, err := r.hostWritePath(name)
	if err != nil {
		return err
	}
	if err := os.WriteFile(host, data, perm); err != nil {
		return rootedPathError("write", name, err)
	}
	return nil
}

func (r *Rooted) AppendFile(name string, data []byte, perm iofs.FileMode) error {
	host, err := r.hostWritePath(name)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(host, os.O_CREATE|os.O_WRONLY|os.O_APPEND, perm)
	if err != nil {
		return rootedPathError("append", name, err)
	}
	defer file.Close()
	_, err = file.Write(data)
	if err != nil {
		return rootedPathError("append", name, err)
	}
	return nil
}

func (r *Rooted) Mkdir(name string, perm iofs.FileMode) error {
	host, err := r.hostCreatePath(name)
	if err != nil {
		return err
	}
	if err := os.Mkdir(host, perm); err != nil {
		return rootedPathError("mkdir", name, err)
	}
	return nil
}

func (r *Rooted) MkdirAll(name string, perm iofs.FileMode) error {
	name = Name(name)
	if name == "." {
		return nil
	}
	parts := strings.Split(name, "/")
	cur := "."
	for _, part := range parts {
		if cur == "." {
			cur = part
		} else {
			cur += "/" + part
		}
		err := r.Mkdir(cur, perm)
		if err != nil && !errors.Is(err, iofs.ErrExist) && !os.IsExist(err) {
			return err
		}
	}
	return nil
}

func (r *Rooted) Remove(name string) error {
	host, err := r.lexicalHostPath(name)
	if err != nil {
		return err
	}
	if err := os.Remove(host); err != nil {
		return rootedPathError("remove", name, err)
	}
	return nil
}

func (r *Rooted) RemoveAll(name string) error {
	name = Name(name)
	if name == "." {
		return errors.New("cannot remove root")
	}
	host, err := r.lexicalHostPath(name)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(host); err != nil {
		return rootedPathError("removeall", name, err)
	}
	return nil
}

func (r *Rooted) Rename(oldName, newName string) error {
	oldHost, err := r.lexicalHostPath(oldName)
	if err != nil {
		return err
	}
	newHost, err := r.hostCreatePath(newName)
	if err != nil {
		return err
	}
	if err := os.Rename(oldHost, newHost); err != nil {
		return rootedPathError("rename", oldName, err)
	}
	return nil
}

func (r *Rooted) Symlink(target, name string) error {
	newHost, err := r.hostCreatePath(name)
	if err != nil {
		return err
	}
	if filepath.IsAbs(target) {
		// The capability API uses virtual absolute targets. Store their rooted
		// host representation so ordinary host filesystem traversal remains
		// functional; Readlink translates it back before exposing it.
		target, err = r.lexicalHostPath(Name(filepath.ToSlash(target)))
		if err != nil {
			return err
		}
	}
	if err := os.Symlink(target, newHost); err != nil {
		return rootedPathError("symlink", name, err)
	}
	return nil
}

func (r *Rooted) GlobalSymlink(target, name string) error {
	newHost, err := r.hostCreatePath(name)
	if err != nil {
		return err
	}
	if target == "" || target[0] != '/' {
		return iofs.ErrInvalid
	}
	// Store global targets as an encoded relative marker. This distinguishes
	// capability-created virtual links from pre-existing absolute host escape
	// links without making the backing host follow the virtual target.
	stored := rootedGlobalSymlinkPrefix + base64.RawURLEncoding.EncodeToString([]byte(target))
	if err := os.Symlink(stored, newHost); err != nil {
		return rootedPathError("symlink", name, err)
	}
	return nil
}

func (r *Rooted) Link(oldName, newName string) error {
	oldHost, err := r.hostPath(oldName, true)
	if err != nil {
		return err
	}
	newHost, err := r.hostCreatePath(newName)
	if err != nil {
		return err
	}
	if err := os.Link(oldHost, newHost); err != nil {
		return rootedPathError("link", oldName, err)
	}
	return nil
}

func (r *Rooted) Chmod(name string, mode iofs.FileMode) error {
	host, err := r.hostPath(name, true)
	if err != nil {
		return err
	}
	if err := os.Chmod(host, mode); err != nil {
		return rootedPathError("chmod", name, err)
	}
	return nil
}

func (r *Rooted) Chtimes(name string, atime, mtime time.Time) error {
	host, err := r.hostPath(name, true)
	if err != nil {
		return err
	}
	if err := os.Chtimes(host, atime, mtime); err != nil {
		return rootedPathError("chtimes", name, err)
	}
	return nil
}

func (r *Rooted) hostWritePath(name string) (string, error) {
	host, err := r.hostPath(name, true)
	if err == nil {
		return host, nil
	}
	if !errors.Is(err, iofs.ErrNotExist) && !os.IsNotExist(err) {
		return "", err
	}
	return r.hostCreatePath(name)
}

func (r *Rooted) hostCreatePath(name string) (string, error) {
	name = Name(name)
	if name == "." {
		return "", iofs.ErrInvalid
	}
	parent := filepath.Dir(name)
	parentHost, err := r.hostPath(parent, true)
	if err != nil {
		return "", err
	}
	return filepath.Join(parentHost, filepath.Base(name)), nil
}

func (r *Rooted) hostPath(name string, followFinal bool) (string, error) {
	host, err := r.lexicalHostPath(name)
	if err != nil {
		return "", err
	}
	var resolved string
	if followFinal {
		resolved, err = filepath.EvalSymlinks(host)
	} else {
		resolved, err = filepath.EvalSymlinks(filepath.Dir(host))
		if err == nil {
			resolved = filepath.Join(resolved, filepath.Base(host))
		}
	}
	if err != nil {
		return "", rootedPathError("resolve", name, err)
	}
	if !r.inside(resolved) {
		return "", iofs.ErrPermission
	}
	return resolved, nil
}

func rootedPathError(op, name string, err error) error {
	if err == nil {
		return nil
	}
	var pathError *iofs.PathError
	if errors.As(err, &pathError) {
		err = pathError.Err
	}
	var linkError *os.LinkError
	if errors.As(err, &linkError) {
		err = linkError.Err
	}
	return &iofs.PathError{Op: op, Path: Name(name), Err: err}
}

func (r *Rooted) lexicalHostPath(name string) (string, error) {
	if name == "" || strings.HasPrefix(name, "/") || !iofs.ValidPath(name) {
		return "", &iofs.PathError{Op: "open", Path: name, Err: iofs.ErrInvalid}
	}
	host := filepath.Join(r.root, filepath.FromSlash(name))
	clean := filepath.Clean(host)
	if !r.inside(clean) {
		return "", iofs.ErrPermission
	}
	return clean, nil
}

func (r *Rooted) inside(host string) bool {
	rel, err := filepath.Rel(r.root, host)
	return err == nil && (rel == "." || rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

func IsCrossDevice(err error) bool {
	return errors.Is(err, syscall.EXDEV)
}

var (
	_ iofs.FS         = (*Rooted)(nil)
	_ iofs.ReadFileFS = (*Rooted)(nil)
	_ iofs.ReadDirFS  = (*Rooted)(nil)
	_ iofs.StatFS     = (*Rooted)(nil)
)
