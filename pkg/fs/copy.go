package fs

import (
	"errors"
	"fmt"
	iofs "io/fs"
	"path"
	"sort"
	"time"
)

// MountPolicy decides what a copy mount does when its target already holds
// data. Copy mounts are not routed like Mountable mounts: the source tree is
// copied into the destination filesystem once, so a database-backed filesystem
// keeps the data after the source disappears.
type MountPolicy int

const (
	// MountIfAbsent copies only when the target is missing or is an empty
	// directory. It is the default so that repeated runs against a persisted
	// database never overwrite work done inside the shell.
	MountIfAbsent MountPolicy = iota
	// MountMerge copies over an existing target, replacing files that the
	// source also provides and leaving every other entry alone.
	MountMerge
	// MountReplace removes the target and copies the source into it.
	MountReplace
)

// CopyMount describes a source tree copied into a filesystem.
type CopyMount struct {
	// Source is any io/fs implementation. It takes precedence over HostPath.
	Source iofs.FS
	// HostPath is a host directory exposed through Rooted, which rejects
	// symbolic links escaping the directory.
	HostPath string
	// Target is the absolute virtual path the source is copied to. It
	// defaults to "/".
	Target string
	// Policy decides what happens when the target already holds data.
	Policy MountPolicy
}

// ApplyMounts copies every mount into destination in order.
func ApplyMounts(destination iofs.FS, mounts []CopyMount) error {
	for _, mount := range mounts {
		if err := ApplyMount(destination, mount); err != nil {
			return err
		}
	}
	return nil
}

// ApplyMount copies one source tree into destination according to its policy.
func ApplyMount(destination iofs.FS, mount CopyMount) error {
	source, err := mount.source()
	if err != nil {
		return err
	}
	target := Name(mount.Target)
	populated, err := hasContent(destination, target)
	if err != nil {
		return err
	}
	switch mount.Policy {
	case MountIfAbsent:
		if populated {
			return nil
		}
	case MountMerge:
	case MountReplace:
		if populated {
			if err := clearTarget(destination, target); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("unknown mount policy %d", mount.Policy)
	}
	return CopyTree(destination, target, source)
}

func (m CopyMount) source() (iofs.FS, error) {
	if m.Source != nil {
		return m.Source, nil
	}
	if m.HostPath == "" {
		return nil, errors.New("mount requires Source or HostPath")
	}
	return NewRooted(m.HostPath)
}

// hasContent reports whether target already holds data. An existing but empty
// directory counts as free so that mounting onto the root stays useful.
func hasContent(destination iofs.FS, target string) (bool, error) {
	info, err := Lstat(destination, target)
	if err != nil {
		if errors.Is(err, iofs.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	if !info.IsDir() {
		return true, nil
	}
	entries, err := ReadDir(destination, target)
	if err != nil {
		return false, err
	}
	return len(entries) > 0, nil
}

func clearTarget(destination iofs.FS, target string) error {
	if target != rootPath {
		return RemoveAll(destination, target)
	}
	entries, err := ReadDir(destination, target)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := RemoveAll(destination, entry.Name()); err != nil {
			return err
		}
	}
	return nil
}

// CopyTree copies the whole source filesystem into target inside destination.
// Directories, permissions, modification times, symbolic links and file data
// are preserved as far as destination's capabilities allow.
func CopyTree(destination iofs.FS, target string, source iofs.FS) error {
	copier := treeCopier{destination: destination, source: source, root: Name(target)}
	info, err := Lstat(source, rootPath)
	if err != nil {
		return err
	}
	if err := MkdirAll(destination, copier.root, info.Mode().Perm()); err != nil {
		return err
	}
	return copier.directory(rootPath)
}

type treeCopier struct {
	destination iofs.FS
	source      iofs.FS
	root        string
}

func (c treeCopier) target(name string) string {
	if name == rootPath {
		return c.root
	}
	if c.root == rootPath {
		return name
	}
	return path.Join(c.root, name)
}

func (c treeCopier) directory(name string) error {
	entries, err := ReadDir(c.source, name)
	if err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		if err := c.entry(path.Join(name, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func (c treeCopier) entry(name string) error {
	target := c.target(name)
	info, err := Lstat(c.source, name)
	if err != nil {
		return err
	}
	switch {
	case info.Mode()&iofs.ModeSymlink != 0:
		return c.symlink(name, target)
	case info.IsDir():
		if err := MkdirAll(c.destination, target, info.Mode().Perm()); err != nil {
			return err
		}
		if err := c.directory(name); err != nil {
			return err
		}
	default:
		data, err := ReadFile(c.source, name)
		if err != nil {
			return err
		}
		if err := WriteFile(c.destination, target, data, info.Mode().Perm()); err != nil {
			return err
		}
	}
	return c.metadata(target, info)
}

func (c treeCopier) symlink(name, target string) error {
	link, err := VirtualReadlink(c.source, name)
	if err != nil {
		return err
	}
	// Absolute targets are absolute inside the source, so they are rebased
	// onto the mount point instead of the destination root.
	if len(link) > 0 && link[0] == '/' && c.root != rootPath {
		link = "/" + path.Join(c.root, link)
	}
	if err := RemoveAll(c.destination, target); err != nil && !errors.Is(err, ErrReadOnly) {
		return err
	}
	return Symlink(c.destination, link, target)
}

// metadata restores permissions and modification time. Destinations lacking
// those capabilities keep the copied data rather than failing the copy.
func (c treeCopier) metadata(target string, info iofs.FileInfo) error {
	if err := Chmod(c.destination, target, info.Mode().Perm()); err != nil && !errors.Is(err, ErrReadOnly) {
		return err
	}
	modified := info.ModTime()
	if modified.IsZero() {
		modified = time.Now()
	}
	if err := Chtimes(c.destination, target, modified, modified); err != nil && !errors.Is(err, ErrReadOnly) {
		return err
	}
	return nil
}
