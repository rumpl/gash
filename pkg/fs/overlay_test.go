package fs

import (
	"bytes"
	"errors"
	iofs "io/fs"
	"testing"
	"testing/fstest"
	"time"
)

func newTestOverlay(t *testing.T, lower iofs.FS) (*Overlay, *Memory) {
	t.Helper()
	upper := NewMemory(1 << 20)
	o, err := NewOverlay(OverlayOptions{Upper: upper, Lower: lower})
	if err != nil {
		t.Fatal(err)
	}
	return o, upper
}

func TestOverlayCopiesUpLowerFileBeforeMutating(t *testing.T) {
	lower := fstest.MapFS{
		"dir/notes.txt": {Data: []byte("lower\n"), Mode: 0o640},
	}
	o, upper := newTestOverlay(t, lower)

	if err := o.AppendFile("dir/notes.txt", []byte("appended\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := o.ReadFile("dir/notes.txt")
	if err != nil || string(got) != "lower\nappended\n" {
		t.Fatalf("append after copy-up got %q err=%v", got, err)
	}
	if got, err = upper.ReadFile("dir/notes.txt"); err != nil || string(got) != "lower\nappended\n" {
		t.Fatalf("upper copy got %q err=%v", got, err)
	}
	if got, err = iofs.ReadFile(lower, "dir/notes.txt"); err != nil || string(got) != "lower\n" {
		t.Fatalf("lower mutated: got %q err=%v", got, err)
	}
	info, err := upper.Stat("dir/notes.txt")
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("copy-up mode=%v", info.Mode().Perm())
	}
}

func TestOverlayCopiesUpForMetadataMutations(t *testing.T) {
	lower := fstest.MapFS{
		"a.txt":    {Data: []byte("content"), Mode: 0o644},
		"dir/keep": {Data: []byte("keep")},
	}
	o, upper := newTestOverlay(t, lower)

	if err := o.Chmod("a.txt", 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := upper.Stat("a.txt")
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("chmod copy-up info=%v err=%v", info, err)
	}
	data, err := upper.ReadFile("a.txt")
	if err != nil || string(data) != "content" {
		t.Fatalf("chmod copy-up lost content: %q %v", data, err)
	}

	stamp := time.Date(2020, 3, 4, 5, 6, 7, 0, time.UTC)
	if err := o.Chtimes("dir/keep", stamp, stamp); err != nil {
		t.Fatal(err)
	}
	if info, err = upper.Stat("dir/keep"); err != nil || !info.ModTime().Equal(stamp) {
		t.Fatalf("chtimes copy-up info=%v err=%v", info, err)
	}
}

func TestOverlayWhiteoutHidesLowerEntry(t *testing.T) {
	lower := fstest.MapFS{
		"dir/gone.txt": {Data: []byte("lower")},
		"dir/keep.txt": {Data: []byte("keep")},
	}
	o, upper := newTestOverlay(t, lower)

	if err := o.Remove("dir/gone.txt"); err != nil {
		t.Fatalf("remove lower-only entry: %v", err)
	}
	if _, err := o.ReadFile("dir/gone.txt"); !errors.Is(err, iofs.ErrNotExist) {
		t.Fatalf("removed entry still readable: %v", err)
	}
	if _, err := o.Stat("dir/gone.txt"); !errors.Is(err, iofs.ErrNotExist) {
		t.Fatalf("removed entry still stattable: %v", err)
	}
	if _, err := o.Open("dir/gone.txt"); !errors.Is(err, iofs.ErrNotExist) {
		t.Fatalf("removed entry still openable: %v", err)
	}
	entries, err := o.ReadDir("dir")
	if err != nil || names(entries) != "keep.txt" {
		t.Fatalf("entries=%s err=%v", names(entries), err)
	}
	if _, err := iofs.ReadFile(lower, "dir/gone.txt"); err != nil {
		t.Fatalf("lower layer mutated: %v", err)
	}
	if _, err := upper.Stat("dir/.wh.gone.txt"); err != nil {
		t.Fatalf("expected persisted whiteout marker: %v", err)
	}
	if err := o.Remove("dir/gone.txt"); !errors.Is(err, iofs.ErrNotExist) {
		t.Fatalf("second remove: %v", err)
	}
}

func TestOverlayHidesWhiteoutMarkersFromListings(t *testing.T) {
	lower := fstest.MapFS{"gone.txt": {Data: []byte("lower")}}
	o, _ := newTestOverlay(t, lower)
	if err := o.Remove("gone.txt"); err != nil {
		t.Fatal(err)
	}
	if err := o.WriteFile("kept.txt", []byte("kept"), 0o644); err != nil {
		t.Fatal(err)
	}
	entries, err := o.ReadDir(".")
	if err != nil || names(entries) != "kept.txt" {
		t.Fatalf("readdir entries=%s err=%v", names(entries), err)
	}
	dir, err := o.Open(".")
	if err != nil {
		t.Fatal(err)
	}
	defer dir.Close()
	reader, ok := dir.(iofs.ReadDirFile)
	if !ok {
		t.Fatalf("open directory is not a ReadDirFile: %T", dir)
	}
	entries, err = reader.ReadDir(-1)
	if err != nil || names(entries) != "kept.txt" {
		t.Fatalf("opened directory entries=%s err=%v", names(entries), err)
	}
	if _, err := o.ReadFile(".wh.gone.txt"); !errors.Is(err, iofs.ErrInvalid) {
		t.Fatalf("marker reachable by name: %v", err)
	}
	if err := o.WriteFile(".wh.kept.txt", nil, 0o644); !errors.Is(err, iofs.ErrInvalid) {
		t.Fatalf("marker writable by name: %v", err)
	}
}

func TestOverlayRecreatingRemovedFileClearsWhiteout(t *testing.T) {
	lower := fstest.MapFS{"file": {Data: []byte("lower")}}
	o, upper := newTestOverlay(t, lower)
	if err := o.Remove("file"); err != nil {
		t.Fatal(err)
	}
	if err := o.AppendFile("file", []byte("fresh"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := o.ReadFile("file")
	if err != nil || string(got) != "fresh" {
		t.Fatalf("recreated file got %q err=%v", got, err)
	}
	if _, err := upper.Stat(".wh.file"); !errors.Is(err, iofs.ErrNotExist) {
		t.Fatalf("marker not cleared: %v", err)
	}
	if err := o.Remove("file"); err != nil {
		t.Fatal(err)
	}
	if _, err := o.ReadFile("file"); !errors.Is(err, iofs.ErrNotExist) {
		t.Fatalf("lower entry resurfaced: %v", err)
	}
}

func TestOverlayRemovedDirectoryStaysOpaque(t *testing.T) {
	lower := fstest.MapFS{
		"dir/a":     {Data: []byte("a")},
		"dir/sub/b": {Data: []byte("b")},
	}
	o, _ := newTestOverlay(t, lower)

	if err := o.RemoveAll("dir"); err != nil {
		t.Fatal(err)
	}
	if _, err := o.Stat("dir/sub/b"); !errors.Is(err, iofs.ErrNotExist) {
		t.Fatalf("descendant survived removal: %v", err)
	}
	if err := o.Mkdir("dir", 0o755); err != nil {
		t.Fatal(err)
	}
	entries, err := o.ReadDir("dir")
	if err != nil || len(entries) != 0 {
		t.Fatalf("recreated directory entries=%s err=%v", names(entries), err)
	}
	if err := o.WriteFile("dir/c", []byte("c"), 0o644); err != nil {
		t.Fatal(err)
	}
	if entries, err = o.ReadDir("dir"); err != nil || names(entries) != "c" {
		t.Fatalf("entries=%s err=%v", names(entries), err)
	}
	if err := o.Remove("dir/c"); err != nil {
		t.Fatal(err)
	}
	if entries, err = o.ReadDir("dir"); err != nil || len(entries) != 0 {
		t.Fatalf("entries=%s err=%v", names(entries), err)
	}
}

func TestOverlayDirectoryRemovalUsesMergedEmptiness(t *testing.T) {
	lower := fstest.MapFS{"dir/a": {Data: []byte("a")}}
	o, _ := newTestOverlay(t, lower)

	if err := o.Remove("dir"); !errors.Is(err, ErrNotEmpty) {
		t.Fatalf("expected merged non-empty directory, got %v", err)
	}
	if err := o.Remove("dir/a"); err != nil {
		t.Fatal(err)
	}
	if err := o.Remove("dir"); err != nil {
		t.Fatalf("remove emptied directory: %v", err)
	}
	if _, err := o.Stat("dir"); !errors.Is(err, iofs.ErrNotExist) {
		t.Fatalf("directory survived: %v", err)
	}
	if err := o.Mkdir("dir", 0o755); err != nil {
		t.Fatal(err)
	}
	entries, err := o.ReadDir("dir")
	if err != nil || len(entries) != 0 {
		t.Fatalf("entries=%s err=%v", names(entries), err)
	}
}

func TestOverlayRenameCopiesUpAndWhitesOutSource(t *testing.T) {
	lower := fstest.MapFS{
		"file":       {Data: []byte("file")},
		"tree/a":     {Data: []byte("a")},
		"tree/sub/b": {Data: []byte("b")},
	}
	o, upper := newTestOverlay(t, lower)

	if err := o.Rename("file", "renamed"); err != nil {
		t.Fatal(err)
	}
	got, err := o.ReadFile("renamed")
	if err != nil || string(got) != "file" {
		t.Fatalf("renamed file got %q err=%v", got, err)
	}
	if _, err := o.Stat("file"); !errors.Is(err, iofs.ErrNotExist) {
		t.Fatalf("rename source visible: %v", err)
	}

	if err := o.Rename("tree", "moved"); err != nil {
		t.Fatal(err)
	}
	if got, err = o.ReadFile("moved/sub/b"); err != nil || string(got) != "b" {
		t.Fatalf("renamed tree got %q err=%v", got, err)
	}
	if _, err := o.Stat("tree/a"); !errors.Is(err, iofs.ErrNotExist) {
		t.Fatalf("renamed tree source visible: %v", err)
	}
	if got, err = upper.ReadFile("moved/a"); err != nil || string(got) != "a" {
		t.Fatalf("upper tree copy got %q err=%v", got, err)
	}
	if _, err := iofs.ReadFile(lower, "tree/a"); err != nil {
		t.Fatalf("lower layer mutated: %v", err)
	}
}

func TestOverlayCopiesUpLowerSymlink(t *testing.T) {
	backing := NewMemory(0)
	if err := backing.MkdirAll("dir", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := backing.WriteFile("dir/target", []byte("target"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := backing.Symlink("target", "dir/sym"); err != nil {
		t.Fatal(err)
	}
	o, upper := newTestOverlay(t, ReadOnly(backing))

	if err := o.Rename("dir/sym", "dir/moved"); err != nil {
		t.Fatal(err)
	}
	target, err := upper.Readlink("dir/moved")
	if err != nil || target != "target" {
		t.Fatalf("copied symlink target=%q err=%v", target, err)
	}
	if _, err := o.Lstat("dir/sym"); !errors.Is(err, iofs.ErrNotExist) {
		t.Fatalf("renamed symlink source visible: %v", err)
	}
	if target, err = backing.Readlink("dir/sym"); err != nil || target != "target" {
		t.Fatalf("lower symlink mutated: %q %v", target, err)
	}
}

func TestOverlayLinkAndSymlinkAcrossLayers(t *testing.T) {
	lower := fstest.MapFS{"data/file": {Data: []byte("payload")}}
	o, upper := newTestOverlay(t, lower)

	if err := o.Link("data/file", "data/hard"); err != nil {
		t.Fatal(err)
	}
	got, err := o.ReadFile("data/hard")
	if err != nil || string(got) != "payload" {
		t.Fatalf("hard link got %q err=%v", got, err)
	}
	if _, err := upper.Stat("data/file"); err != nil {
		t.Fatalf("hard link did not copy up source: %v", err)
	}
	if err := o.Symlink("file", "data/soft"); err != nil {
		t.Fatal(err)
	}
	target, err := o.Readlink("data/soft")
	if err != nil || target != "file" {
		t.Fatalf("readlink got %q err=%v", target, err)
	}
	if got, err = o.ReadFile("data/soft"); err != nil || !bytes.Equal(got, []byte("payload")) {
		t.Fatalf("symlink read got %q err=%v", got, err)
	}
	if err := o.Symlink("file", "data/soft"); !errors.Is(err, iofs.ErrExist) {
		t.Fatalf("expected existing symlink error, got %v", err)
	}
}

func TestOverlayMkdirRespectsLowerEntries(t *testing.T) {
	lower := fstest.MapFS{"dir/file": {Data: []byte("x")}}
	o, upper := newTestOverlay(t, lower)

	if err := o.Mkdir("dir", 0o755); !errors.Is(err, iofs.ErrExist) {
		t.Fatalf("expected existing lower directory, got %v", err)
	}
	if err := o.MkdirAll("dir/deep/nested", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := o.WriteFile("dir/deep/nested/file", []byte("deep"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := upper.ReadFile("dir/deep/nested/file")
	if err != nil || string(got) != "deep" {
		t.Fatalf("nested write got %q err=%v", got, err)
	}
	entries, err := o.ReadDir("dir")
	if err != nil || names(entries) != "deep,file" {
		t.Fatalf("entries=%s err=%v", names(entries), err)
	}
	if err := o.MkdirAll("dir/file/child", 0o755); !errors.Is(err, ErrNotDir) {
		t.Fatalf("expected not-a-directory, got %v", err)
	}
}

func TestOverlayReadOnlyUpperReportsReadOnly(t *testing.T) {
	lower := fstest.MapFS{"file": {Data: []byte("lower")}}
	o, err := NewOverlay(OverlayOptions{Upper: fstest.MapFS{}, Lower: lower})
	if err != nil {
		t.Fatal(err)
	}
	if err := o.Remove("file"); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("expected read-only upper, got %v", err)
	}
	if err := o.AppendFile("file", []byte("x"), 0o644); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("expected read-only upper, got %v", err)
	}
	got, err := o.ReadFile("file")
	if err != nil || string(got) != "lower" {
		t.Fatalf("lower read got %q err=%v", got, err)
	}
}

func TestOverlayMergedViewThroughStandardHelpers(t *testing.T) {
	lower := fstest.MapFS{
		"dir/lower.txt": {Data: []byte("lower")},
		"dir/gone.txt":  {Data: []byte("gone")},
	}
	o, _ := newTestOverlay(t, lower)
	if err := o.Remove("dir/gone.txt"); err != nil {
		t.Fatal(err)
	}
	if err := o.WriteFile("dir/upper.txt", []byte("upper"), 0o644); err != nil {
		t.Fatal(err)
	}
	visited := map[string]bool{}
	if err := iofs.WalkDir(o, ".", func(name string, entry iofs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		visited[name] = entry.IsDir()
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{".": true, "dir": true, "dir/lower.txt": false, "dir/upper.txt": false}
	if len(visited) != len(want) {
		t.Fatalf("walk visited %v", visited)
	}
	for name, isDir := range want {
		got, ok := visited[name]
		if !ok || got != isDir {
			t.Fatalf("walk visited %v", visited)
		}
	}
	matches, err := iofs.Glob(o, "dir/*.txt")
	if err != nil || len(matches) != 2 {
		t.Fatalf("glob matches=%v err=%v", matches, err)
	}
}

func TestOverlayPersistsWhiteoutsInHostUpperLayer(t *testing.T) {
	lower := fstest.MapFS{
		"state/keep.txt": {Data: []byte("keep")},
		"state/drop.txt": {Data: []byte("drop")},
	}
	upperRoot := t.TempDir()
	newOverlay := func() *Overlay {
		t.Helper()
		upper, err := NewRooted(upperRoot)
		if err != nil {
			t.Fatal(err)
		}
		o, err := NewOverlay(OverlayOptions{Upper: upper, Lower: lower})
		if err != nil {
			t.Fatal(err)
		}
		return o
	}

	first := newOverlay()
	if err := first.Remove("state/drop.txt"); err != nil {
		t.Fatal(err)
	}
	if err := first.AppendFile("state/keep.txt", []byte("+more"), 0o644); err != nil {
		t.Fatal(err)
	}

	reopened := newOverlay()
	if _, err := reopened.Stat("state/drop.txt"); !errors.Is(err, iofs.ErrNotExist) {
		t.Fatalf("deletion did not persist: %v", err)
	}
	got, err := reopened.ReadFile("state/keep.txt")
	if err != nil || string(got) != "keep+more" {
		t.Fatalf("copy-up did not persist: %q %v", got, err)
	}
	entries, err := reopened.ReadDir("state")
	if err != nil || names(entries) != "keep.txt" {
		t.Fatalf("entries=%s err=%v", names(entries), err)
	}
}
