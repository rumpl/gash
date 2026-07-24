package fs

import (
	"errors"
	"io"
	iofs "io/fs"
	"path"
	"sort"
	"strings"
	"time"
)

// whiteoutPrefix marks an upper-layer entry that hides a lower-layer entry with
// the same base name. The marker is an ordinary file, so any writable upper
// filesystem can persist a deletion without a portable io/fs whiteout
// capability. Entries using the prefix are never visible through the overlay.
const whiteoutPrefix = ".wh."

// Overlay presents a writable upper filesystem over a read-only lower
// filesystem. Reads prefer upper entries and fall back to lower entries.
// Mutations are always directed to upper: modifying an entry that exists only
// in the lower layer copies it up first, and removing such an entry records a
// whiteout marker in upper so the lower entry stays hidden. Removing a
// directory makes it opaque, so a directory recreated at the same path does not
// expose the lower layer's former contents. Copy-up preserves the lower
// permission bits and adds owner access so a host-backed upper layer stays
// writable.
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
	if err := rejectWhiteout("open", name); err != nil {
		return nil, err
	}
	if info, err := o.Stat(name); err == nil && info.IsDir() {
		entries, dirErr := o.ReadDir(name)
		if dirErr != nil {
			return nil, dirErr
		}
		return &overlayDir{info: info, entries: entries}, nil
	}
	if f, err := o.upper.Open(name); err == nil {
		return f, nil
	} else if !isNotExist(err) {
		return nil, err
	}
	if o.whitedOut(name) {
		return nil, notExist("open", name)
	}
	return o.lower.Open(name)
}

func (o *Overlay) ReadFile(name string) ([]byte, error) {
	name = Name(name)
	if err := rejectWhiteout("open", name); err != nil {
		return nil, err
	}
	if data, err := iofs.ReadFile(o.upper, name); err == nil {
		return data, nil
	} else if !isNotExist(err) {
		return nil, err
	}
	if o.whitedOut(name) {
		return nil, notExist("open", name)
	}
	return iofs.ReadFile(o.lower, name)
}

func (o *Overlay) ReadDir(name string) ([]iofs.DirEntry, error) {
	name = Name(name)
	if err := rejectWhiteout("readdir", name); err != nil {
		return nil, err
	}
	merged := map[string]iofs.DirEntry{}
	var lowerEntries []iofs.DirEntry
	lowerErr := error(nil)
	if o.whitedOut(name) {
		lowerErr = notExist("readdir", name)
	} else {
		lowerEntries, lowerErr = iofs.ReadDir(o.lower, name)
		if lowerErr != nil && !isNotExist(lowerErr) {
			return nil, lowerErr
		}
	}
	for _, entry := range lowerEntries {
		if o.hasWhiteout(join(name, entry.Name())) {
			continue
		}
		merged[entry.Name()] = entry
	}
	upperEntries, upperErr := iofs.ReadDir(o.upper, name)
	if upperErr != nil && !isNotExist(upperErr) {
		return nil, upperErr
	}
	for _, entry := range upperEntries {
		if isWhiteoutName(entry.Name()) {
			delete(merged, strings.TrimPrefix(entry.Name(), whiteoutPrefix))
			continue
		}
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
	if err := rejectWhiteout("stat", name); err != nil {
		return nil, err
	}
	if info, err := iofs.Stat(o.upper, name); err == nil {
		return info, nil
	} else if !isNotExist(err) {
		return nil, err
	}
	if o.whitedOut(name) {
		return nil, notExist("stat", name)
	}
	return iofs.Stat(o.lower, name)
}

func (o *Overlay) Lstat(name string) (iofs.FileInfo, error) {
	name = Name(name)
	if err := rejectWhiteout("lstat", name); err != nil {
		return nil, err
	}
	if info, err := Lstat(o.upper, name); err == nil {
		return info, nil
	} else if !isNotExist(err) {
		return nil, err
	}
	if o.whitedOut(name) {
		return nil, notExist("lstat", name)
	}
	return Lstat(o.lower, name)
}

func (o *Overlay) WriteFile(name string, data []byte, perm iofs.FileMode) error {
	name = Name(name)
	if err := o.prepareWrite("open", name); err != nil {
		return err
	}
	if err := WriteFile(o.upper, name, data, perm); err != nil {
		return err
	}
	o.clearWhiteout(name)
	return nil
}

func (o *Overlay) AppendFile(name string, data []byte, perm iofs.FileMode) error {
	name = Name(name)
	if err := o.prepareWrite("open", name); err != nil {
		return err
	}
	if err := o.copyUp(name); err != nil {
		return err
	}
	if err := AppendFile(o.upper, name, data, perm); err != nil {
		return err
	}
	o.clearWhiteout(name)
	return nil
}

func (o *Overlay) Mkdir(name string, perm iofs.FileMode) error {
	name = Name(name)
	if err := o.prepareWrite("mkdir", name); err != nil {
		return err
	}
	if _, err := o.Lstat(name); err == nil {
		return &iofs.PathError{Op: "mkdir", Path: name, Err: iofs.ErrExist}
	}
	// A whiteout for this path is kept so the recreated directory stays opaque.
	return Mkdir(o.upper, name, perm)
}

func (o *Overlay) MkdirAll(name string, perm iofs.FileMode) error {
	name = Name(name)
	if err := rejectWhiteout("mkdir", name); err != nil {
		return err
	}
	return o.materializeDirs(name, perm)
}

func (o *Overlay) Remove(name string) error {
	name = Name(name)
	if name == "." {
		return &iofs.PathError{Op: "remove", Path: name, Err: iofs.ErrInvalid}
	}
	if err := rejectWhiteout("remove", name); err != nil {
		return err
	}
	info, err := o.Lstat(name)
	if err != nil {
		return err
	}
	if info.IsDir() {
		entries, dirErr := o.ReadDir(name)
		if dirErr != nil {
			return dirErr
		}
		if len(entries) > 0 {
			return &iofs.PathError{Op: "remove", Path: name, Err: ErrNotEmpty}
		}
	}
	inUpper := false
	if _, upperErr := Lstat(o.upper, name); upperErr == nil {
		inUpper = true
	} else if !isNotExist(upperErr) {
		return upperErr
	}
	if inUpper {
		if info.IsDir() {
			// Markers inside the directory are redundant once the directory
			// itself is removed, and would block the upper removal.
			if err := o.clearMarkers(name); err != nil {
				return err
			}
		}
		if err := Remove(o.upper, name); err != nil {
			return err
		}
	}
	if !o.visibleInLower(name) {
		return nil
	}
	return o.setWhiteout(name)
}

func (o *Overlay) RemoveAll(name string) error {
	name = Name(name)
	if name == "." {
		return &iofs.PathError{Op: "removeall", Path: name, Err: iofs.ErrInvalid}
	}
	if err := rejectWhiteout("removeall", name); err != nil {
		return err
	}
	hideLower := o.visibleInLower(name)
	if _, upperErr := Lstat(o.upper, name); upperErr == nil {
		if err := RemoveAll(o.upper, name); err != nil {
			return err
		}
	} else if !isNotExist(upperErr) {
		return upperErr
	}
	if !hideLower {
		return nil
	}
	return o.setWhiteout(name)
}

func (o *Overlay) Rename(oldName, newName string) error {
	oldName, newName = Name(oldName), Name(newName)
	if err := rejectWhiteout("rename", oldName); err != nil {
		return err
	}
	if err := rejectWhiteout("rename", newName); err != nil {
		return err
	}
	if _, err := o.Lstat(oldName); err != nil {
		return err
	}
	if err := o.copyUpTree(oldName); err != nil {
		return err
	}
	if err := o.materializeParents(newName); err != nil {
		return err
	}
	if err := Rename(o.upper, oldName, newName); err != nil {
		return err
	}
	if o.visibleInLower(oldName) {
		if err := o.setWhiteout(oldName); err != nil {
			return err
		}
	}
	if info, err := Lstat(o.upper, newName); err == nil && !info.IsDir() {
		o.clearWhiteout(newName)
	}
	return nil
}

func (o *Overlay) Symlink(target, name string) error {
	name = Name(name)
	if err := o.prepareWrite("symlink", name); err != nil {
		return err
	}
	if _, err := o.Lstat(name); err == nil {
		return &iofs.PathError{Op: "symlink", Path: name, Err: iofs.ErrExist}
	}
	if err := Symlink(o.upper, target, name); err != nil {
		return err
	}
	o.clearWhiteout(name)
	return nil
}

func (o *Overlay) Readlink(name string) (string, error) {
	name = Name(name)
	if err := rejectWhiteout("readlink", name); err != nil {
		return "", err
	}
	if target, err := Readlink(o.upper, name); err == nil {
		return target, nil
	} else if !isNotExist(err) {
		return "", err
	}
	if o.whitedOut(name) {
		return "", notExist("readlink", name)
	}
	return Readlink(o.lower, name)
}

func (o *Overlay) Chmod(name string, mode iofs.FileMode) error {
	name = Name(name)
	if err := o.prepareWrite("chmod", name); err != nil {
		return err
	}
	if err := o.copyUp(name); err != nil {
		return err
	}
	return Chmod(o.upper, name, mode)
}

func (o *Overlay) Link(oldName, newName string) error {
	oldName, newName = Name(oldName), Name(newName)
	if err := rejectWhiteout("link", oldName); err != nil {
		return err
	}
	if err := o.prepareWrite("link", newName); err != nil {
		return err
	}
	if err := o.copyUp(oldName); err != nil {
		return err
	}
	if err := Link(o.upper, oldName, newName); err != nil {
		return err
	}
	o.clearWhiteout(newName)
	return nil
}

func (o *Overlay) Chtimes(name string, atime, mtime time.Time) error {
	name = Name(name)
	if err := o.prepareWrite("chtimes", name); err != nil {
		return err
	}
	if err := o.copyUp(name); err != nil {
		return err
	}
	return Chtimes(o.upper, name, atime, mtime)
}

// prepareWrite rejects reserved whiteout names and copies the existing lower
// parent directories of name into the upper layer.
func (o *Overlay) prepareWrite(op, name string) error {
	if err := rejectWhiteout(op, name); err != nil {
		return err
	}
	return o.materializeParents(name)
}

// copyUp copies a lower-only entry into the upper layer so later mutations
// observe the lower content. Entries that are absent or hidden are left alone
// so the mutating operation reports its own error or creates the entry.
func (o *Overlay) copyUp(name string) error {
	if name == "." {
		return nil
	}
	if _, err := Lstat(o.upper, name); err == nil {
		return nil
	} else if !isNotExist(err) {
		return err
	}
	if o.whitedOut(name) {
		return nil
	}
	info, err := Lstat(o.lower, name)
	if err != nil {
		return nil
	}
	if err := o.materializeParents(name); err != nil {
		return err
	}
	switch {
	case info.Mode()&iofs.ModeSymlink != 0:
		target, linkErr := Readlink(o.lower, name)
		if linkErr != nil {
			return linkErr
		}
		return Symlink(o.upper, target, name)
	case info.IsDir():
		if err := Mkdir(o.upper, name, copiedPerm(info.Mode())); err != nil && !errors.Is(err, iofs.ErrExist) {
			return err
		}
		return nil
	default:
		data, readErr := iofs.ReadFile(o.lower, name)
		if readErr != nil {
			return readErr
		}
		if err := WriteFile(o.upper, name, data, copiedPerm(info.Mode())); err != nil {
			return err
		}
		if err := Chtimes(o.upper, name, info.ModTime(), info.ModTime()); err != nil && !errors.Is(err, ErrReadOnly) {
			return err
		}
		return nil
	}
}

// copyUpTree copies an entry and, for directories, its visible descendants.
func (o *Overlay) copyUpTree(name string) error {
	if err := o.copyUp(name); err != nil {
		return err
	}
	info, err := o.Lstat(name)
	if err != nil || !info.IsDir() {
		return err
	}
	entries, err := o.ReadDir(name)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := o.copyUpTree(join(name, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

// materializeParents copies the existing parent directories of name into the
// upper layer without creating parents that do not exist in either layer.
func (o *Overlay) materializeParents(name string) error {
	return o.walkDirs(parentName(name), 0, false)
}

// materializeDirs implements MkdirAll semantics across both layers.
func (o *Overlay) materializeDirs(name string, perm iofs.FileMode) error {
	return o.walkDirs(name, perm.Perm(), true)
}

func (o *Overlay) walkDirs(name string, perm iofs.FileMode, create bool) error {
	if name == "." || name == "" {
		return nil
	}
	current := ""
	for _, part := range strings.Split(name, "/") {
		if current == "" {
			current = part
		} else {
			current += "/" + part
		}
		if info, err := iofs.Stat(o.upper, current); err == nil {
			if !info.IsDir() {
				return &iofs.PathError{Op: "mkdir", Path: current, Err: ErrNotDir}
			}
			continue
		} else if !isNotExist(err) {
			return err
		}
		info, err := o.Stat(current)
		switch {
		case err == nil && info.IsDir():
			err = Mkdir(o.upper, current, copiedPerm(info.Mode()))
		case err == nil:
			return &iofs.PathError{Op: "mkdir", Path: current, Err: ErrNotDir}
		case create && isNotExist(err):
			err = Mkdir(o.upper, current, perm)
		default:
			return err
		}
		if err != nil && !errors.Is(err, iofs.ErrExist) {
			return err
		}
	}
	return nil
}

// visibleInLower reports whether the lower layer still exposes name through the
// overlay, which is the condition for recording a whiteout on deletion.
func (o *Overlay) visibleInLower(name string) bool {
	if o.whitedOut(name) {
		return false
	}
	_, err := Lstat(o.lower, name)
	return err == nil
}

// whitedOut reports whether name or one of its parents is hidden by a whiteout.
func (o *Overlay) whitedOut(name string) bool {
	if name == "." || name == "" {
		return false
	}
	current := ""
	for _, part := range strings.Split(name, "/") {
		if current == "" {
			current = part
		} else {
			current += "/" + part
		}
		if o.hasWhiteout(current) {
			return true
		}
	}
	return false
}

func (o *Overlay) hasWhiteout(name string) bool {
	marker := whiteoutName(name)
	if marker == "" {
		return false
	}
	_, err := Lstat(o.upper, marker)
	return err == nil
}

func (o *Overlay) setWhiteout(name string) error {
	marker := whiteoutName(name)
	if marker == "" {
		return &iofs.PathError{Op: "remove", Path: name, Err: iofs.ErrInvalid}
	}
	if err := o.materializeParents(name); err != nil {
		return err
	}
	if _, err := Lstat(o.upper, marker); err == nil {
		return nil
	}
	return WriteFile(o.upper, marker, nil, 0o600)
}

// clearWhiteout drops the marker for a recreated non-directory entry. Failures
// are not fatal because upper entries already shadow the lower layer.
func (o *Overlay) clearWhiteout(name string) {
	marker := whiteoutName(name)
	if marker == "" {
		return
	}
	if _, err := Lstat(o.upper, marker); err != nil {
		return
	}
	_ = Remove(o.upper, marker)
}

// clearMarkers removes the whiteout markers stored directly in an upper
// directory.
func (o *Overlay) clearMarkers(dir string) error {
	entries, err := iofs.ReadDir(o.upper, dir)
	if err != nil {
		if isNotExist(err) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if !isWhiteoutName(entry.Name()) {
			continue
		}
		if err := Remove(o.upper, join(dir, entry.Name())); err != nil && !isNotExist(err) {
			return err
		}
	}
	return nil
}

// copiedPerm keeps the lower permission bits of a copied-up entry and adds
// owner access. gash does not enforce permission bits in its virtual model, so
// a host-backed upper layer must not become unwritable just because a lower
// entry is read-only.
func copiedPerm(mode iofs.FileMode) iofs.FileMode {
	perm := mode.Perm() | 0o600
	if mode.IsDir() {
		perm |= 0o700
	}
	return perm
}

func whiteoutName(name string) string {
	if name == "." || name == "" {
		return ""
	}
	dir, base := parentName(name), path.Base(name)
	if dir == "." {
		return whiteoutPrefix + base
	}
	return dir + "/" + whiteoutPrefix + base
}

func isWhiteoutName(base string) bool {
	return strings.HasPrefix(base, whiteoutPrefix)
}

func rejectWhiteout(op, name string) error {
	for _, part := range strings.Split(name, "/") {
		if isWhiteoutName(part) {
			return &iofs.PathError{Op: op, Path: name, Err: iofs.ErrInvalid}
		}
	}
	return nil
}

func parentName(name string) string {
	dir := path.Dir(name)
	if dir == "/" || dir == "" {
		return "."
	}
	return dir
}

func join(dir, base string) string {
	if dir == "." || dir == "" {
		return base
	}
	return dir + "/" + base
}

func notExist(op, name string) error {
	return &iofs.PathError{Op: op, Path: name, Err: iofs.ErrNotExist}
}

func isNotExist(err error) bool {
	return errors.Is(err, iofs.ErrNotExist)
}

// overlayDir serves merged directory listings so whiteout markers and shadowed
// lower entries never reach callers that open a directory directly.
type overlayDir struct {
	info    iofs.FileInfo
	entries []iofs.DirEntry
	offset  int
}

func (d *overlayDir) Stat() (iofs.FileInfo, error) {
	return d.info, nil
}

func (d *overlayDir) Read([]byte) (int, error) {
	return 0, ErrIsDir
}

func (d *overlayDir) Close() error {
	return nil
}

func (d *overlayDir) ReadDir(n int) ([]iofs.DirEntry, error) {
	if d.offset >= len(d.entries) {
		if n > 0 {
			return nil, io.EOF
		}
		return []iofs.DirEntry{}, nil
	}
	end := len(d.entries)
	if n > 0 && d.offset+n < end {
		end = d.offset + n
	}
	out := append([]iofs.DirEntry(nil), d.entries[d.offset:end]...)
	d.offset = end
	return out, nil
}

var (
	_ iofs.FS          = (*Overlay)(nil)
	_ iofs.ReadFileFS  = (*Overlay)(nil)
	_ iofs.ReadDirFS   = (*Overlay)(nil)
	_ iofs.StatFS      = (*Overlay)(nil)
	_ iofs.ReadDirFile = (*overlayDir)(nil)
	_ AppendFileFS     = (*Overlay)(nil)
	_ ChmodFS          = (*Overlay)(nil)
	_ ChtimesFS        = (*Overlay)(nil)
	_ LinkFS           = (*Overlay)(nil)
	_ LstatFS          = (*Overlay)(nil)
	_ MkdirAllFS       = (*Overlay)(nil)
	_ MkdirFS          = (*Overlay)(nil)
	_ ReadlinkFS       = (*Overlay)(nil)
	_ RemoveAllFS      = (*Overlay)(nil)
	_ RemoveFS         = (*Overlay)(nil)
	_ RenameFS         = (*Overlay)(nil)
	_ SymlinkFS        = (*Overlay)(nil)
	_ WriteFileFS      = (*Overlay)(nil)
)
