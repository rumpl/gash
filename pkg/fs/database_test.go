package fs

import (
	"errors"
	iofs "io/fs"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"testing/fstest"
	"time"
)

func newSQLiteTestFS(t *testing.T) *Database {
	t.Helper()
	filesystem, err := NewSQLite(SQLiteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = filesystem.Close() })
	return filesystem
}

func TestSQLiteDatabaseSuite(t *testing.T) {
	runDatabaseSuite(t, func(t *testing.T) *Database {
		t.Helper()
		return newSQLiteTestFS(t)
	})
}

// runDatabaseSuite exercises every capability against a backend so the SQLite
// and Turso filesystems are held to the same behavior as fs.Memory.
func runDatabaseSuite(t *testing.T, open func(t *testing.T) *Database) {
	t.Helper()
	t.Run("StandardFS", func(t *testing.T) {
		d := open(t)
		if err := d.MkdirAll("home/user", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := d.WriteFile("home/user/a", []byte("hello"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := d.AppendFile("home/user/a", []byte(" world"), 0o644); err != nil {
			t.Fatal(err)
		}
		got, err := iofs.ReadFile(d, "home/user/a")
		if err != nil || string(got) != "hello world" {
			t.Fatalf("got %q, %v", got, err)
		}
		entries, err := iofs.ReadDir(d, "home/user")
		if err != nil || len(entries) != 1 || entries[0].Name() != "a" {
			t.Fatalf("entries=%v err=%v", entries, err)
		}
		if err := fstest.TestFS(d, "home/user/a"); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("Lifecycle", func(t *testing.T) {
		d := open(t)
		if err := d.MkdirAll("home/user", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := d.CreateFile("home/user/a", []byte("hello"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := d.CreateFile("home/user/a", []byte("hello"), 0o644); !errors.Is(err, iofs.ErrExist) {
			t.Fatalf("expected exist, got %v", err)
		}
		if err := d.Rename("home/user/a", "home/user/b"); err != nil {
			t.Fatal(err)
		}
		if _, err := d.ReadFile("home/user/a"); !errors.Is(err, iofs.ErrNotExist) {
			t.Fatalf("old path: %v", err)
		}
		got, err := d.ReadFile("home/user/b")
		if err != nil || string(got) != "hello" {
			t.Fatalf("got %q, %v", got, err)
		}
	})
	t.Run("RenameDirectory", func(t *testing.T) {
		d := open(t)
		if err := d.MkdirAll("a/b/c", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := d.WriteFile("a/b/c/f", []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := d.Rename("a/b", "a/moved"); err != nil {
			t.Fatal(err)
		}
		if got, err := d.ReadFile("a/moved/c/f"); err != nil || string(got) != "x" {
			t.Fatalf("got %q, %v", got, err)
		}
		entries, err := d.ReadDir("a")
		if err != nil || len(entries) != 1 || entries[0].Name() != "moved" {
			t.Fatalf("entries=%v err=%v", entries, err)
		}
	})
	t.Run("SymlinksAndQuota", func(t *testing.T) {
		d := open(t)
		if err := d.Mkdir("data", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := d.WriteFile("data/a", []byte("hello"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := d.Symlink("a", "data/link"); err != nil {
			t.Fatal(err)
		}
		got, err := d.ReadFile("data/link")
		if err != nil || string(got) != "hello" {
			t.Fatalf("got %q, %v", got, err)
		}
		target, err := d.Readlink("data/link")
		if err != nil || target != "a" {
			t.Fatalf("target=%q err=%v", target, err)
		}
		info, err := d.Lstat("data/link")
		if err != nil || info.Mode()&iofs.ModeSymlink == 0 {
			t.Fatalf("lstat mode=%v err=%v", info, err)
		}
		if err := d.Symlink("loop", "data/loop"); err != nil {
			t.Fatal(err)
		}
		if _, err := d.ReadFile("data/loop"); err == nil {
			t.Fatal("expected an error for a self-referencing link")
		}
	})
	t.Run("Quota", func(t *testing.T) {
		d := open(t)
		d.limit = 5
		if err := d.WriteFile("a", []byte("hello"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := d.AppendFile("a", []byte("!"), 0o644); !errors.Is(err, ErrQuota) {
			t.Fatalf("expected quota, got %v", err)
		}
		if err := d.WriteFile("b", []byte("x"), 0o644); !errors.Is(err, ErrQuota) {
			t.Fatalf("expected quota, got %v", err)
		}
	})
	t.Run("HardLinksAndMetadata", func(t *testing.T) {
		d := open(t)
		if err := d.WriteFile("a", []byte("one"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := d.Link("a", "b"); err != nil {
			t.Fatal(err)
		}
		if err := d.AppendFile("b", []byte("two"), 0o644); err != nil {
			t.Fatal(err)
		}
		got, _ := d.ReadFile("a")
		if string(got) != "onetwo" {
			t.Fatalf("hard link content=%q", got)
		}
		if err := d.Chmod("a", 0o600); err != nil {
			t.Fatal(err)
		}
		info, err := d.Stat("b")
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("mode=%v err=%v", info, err)
		}
		if info.Size() != 6 {
			t.Fatalf("size=%d", info.Size())
		}
		stamp := time.Unix(1700000000, 0)
		if err := d.Chtimes("a", stamp, stamp); err != nil {
			t.Fatal(err)
		}
		info, err = d.Stat("a")
		if err != nil || !info.ModTime().Equal(stamp) {
			t.Fatalf("mtime=%v err=%v", info.ModTime(), err)
		}
		if err := d.Remove("a"); err != nil {
			t.Fatal(err)
		}
		if d.Used() != 6 {
			t.Fatalf("linked bytes released early: %d", d.Used())
		}
		if err := d.Remove("b"); err != nil {
			t.Fatal(err)
		}
		if d.Used() != 0 {
			t.Fatalf("bytes retained: %d", d.Used())
		}
	})
	t.Run("RecursiveRemove", func(t *testing.T) {
		d := open(t)
		if err := d.MkdirAll("a/b", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := d.WriteFile("a/b/f", []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := d.Remove("a"); !errors.Is(err, ErrNotEmpty) {
			t.Fatalf("expected not empty, got %v", err)
		}
		if err := d.RemoveAll("a"); err != nil {
			t.Fatal(err)
		}
		if _, err := d.Stat("a"); !errors.Is(err, iofs.ErrNotExist) {
			t.Fatalf("expected not exist, got %v", err)
		}
		if d.Used() != 0 {
			t.Fatalf("used=%d", d.Used())
		}
		if err := d.RemoveAll("a"); err != nil {
			t.Fatalf("removing a missing tree: %v", err)
		}
	})
	t.Run("BinaryData", func(t *testing.T) {
		d := open(t)
		data := []byte{0x00, 0x01, 0xff, 0xfe, '\n', 0x00}
		if err := d.WriteFile("blob.bin", data, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := d.AppendFile("blob.bin", []byte{0x00, 0x7f}, 0o644); err != nil {
			t.Fatal(err)
		}
		got, err := d.ReadFile("blob.bin")
		if err != nil || string(got) != string(append(data, 0x00, 0x7f)) {
			t.Fatalf("got %v, %v", got, err)
		}
		info, err := d.Stat("blob.bin")
		if err != nil || info.Size() != int64(len(data)+2) {
			t.Fatalf("size=%v err=%v", info, err)
		}
	})
	t.Run("EmptyAndNilData", func(t *testing.T) {
		d := open(t)
		if err := d.WriteFile("empty", nil, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := d.AppendFile("empty", nil, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := d.CreateFile("created", nil, 0o644); err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"empty", "created"} {
			got, err := d.ReadFile(name)
			if err != nil || len(got) != 0 {
				t.Fatalf("%s: %q %v", name, got, err)
			}
			info, err := d.Stat(name)
			if err != nil || info.Size() != 0 {
				t.Fatalf("%s: %v %v", name, info, err)
			}
		}
	})
	t.Run("Errors", func(t *testing.T) {
		d := open(t)
		if err := d.WriteFile("file", []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := d.WriteFile("file/child", []byte("x"), 0o644); !errors.Is(err, ErrNotDir) {
			t.Fatalf("expected not a directory, got %v", err)
		}
		if err := d.Mkdir("missing/child", 0o755); !errors.Is(err, iofs.ErrNotExist) {
			t.Fatalf("expected not exist, got %v", err)
		}
		if err := d.Mkdir("file", 0o755); !errors.Is(err, iofs.ErrExist) {
			t.Fatalf("expected exist, got %v", err)
		}
		if _, err := d.ReadDir("file"); !errors.Is(err, ErrNotDir) {
			t.Fatalf("expected not a directory, got %v", err)
		}
		if err := d.Mkdir("dir", 0o755); err != nil {
			t.Fatal(err)
		}
		if _, err := d.ReadFile("dir"); !errors.Is(err, ErrIsDir) {
			t.Fatalf("expected is a directory, got %v", err)
		}
		if err := d.Remove("."); err == nil {
			t.Fatal("expected removing the root to fail")
		}
	})
}

func TestSQLitePersistsAcrossReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gash.db")
	first, err := NewSQLite(SQLiteOptions{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	if err := first.MkdirAll("project/src", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := first.WriteFile("project/src/main.go", []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := first.Symlink("src/main.go", "project/link"); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	second, err := NewSQLite(SQLiteOptions{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	got, err := second.ReadFile("project/link")
	if err != nil || string(got) != "package main\n" {
		t.Fatalf("got %q, %v", got, err)
	}
	info, err := second.Stat("project/src/main.go")
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("mode=%v err=%v", info, err)
	}
}

func TestSQLiteRejectsInvalidTablePrefix(t *testing.T) {
	if _, err := NewSQLite(SQLiteOptions{Table: "bad name; DROP TABLE"}); err == nil {
		t.Fatal("expected an invalid table prefix to be rejected")
	}
}

func TestSQLiteSeparateTablePrefixes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gash.db")
	first, err := NewSQLite(SQLiteOptions{Path: path, Table: "one"})
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := NewSQLite(SQLiteOptions{Path: path, Table: "two"})
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if err := first.WriteFile("only-in-one", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := second.Stat("only-in-one"); !errors.Is(err, iofs.ErrNotExist) {
		t.Fatalf("prefixes are not isolated: %v", err)
	}
}

func TestDatabaseRequiresDB(t *testing.T) {
	if _, err := NewDatabase(DatabaseOptions{}); err == nil {
		t.Fatal("expected a missing DB to be rejected")
	}
}

func TestDatabaseConcurrentAccess(t *testing.T) {
	d := newSQLiteTestFS(t)
	if err := d.Mkdir("work", 0o755); err != nil {
		t.Fatal(err)
	}
	var group sync.WaitGroup
	for worker := range 8 {
		group.Add(1)
		go func() {
			defer group.Done()
			name := "work/file-" + strconv.Itoa(worker)
			if err := d.WriteFile(name, []byte(strconv.Itoa(worker)), 0o644); err != nil {
				t.Error(err)
				return
			}
			if _, err := d.ReadFile(name); err != nil {
				t.Error(err)
			}
			if _, err := d.ReadDir("work"); err != nil {
				t.Error(err)
			}
			if d.Used() < 0 {
				t.Error("negative usage")
			}
		}()
	}
	group.Wait()
	entries, err := d.ReadDir("work")
	if err != nil || len(entries) != 8 {
		t.Fatalf("entries=%d err=%v", len(entries), err)
	}
}

func TestDatabaseComposesWithOverlayAndMounts(t *testing.T) {
	lower := newSQLiteTestFS(t)
	if err := lower.WriteFile("base.txt", []byte("lower\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	overlay, err := NewOverlay(OverlayOptions{Lower: lower, Upper: NewMemory(0)})
	if err != nil {
		t.Fatal(err)
	}
	if err := overlay.WriteFile("base.txt", []byte("upper\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := overlay.ReadFile("base.txt"); err != nil || string(got) != "upper\n" {
		t.Fatalf("got %q, %v", got, err)
	}
	if got, err := lower.ReadFile("base.txt"); err != nil || string(got) != "lower\n" {
		t.Fatalf("lower changed: %q, %v", got, err)
	}

	mountable, err := NewMountable(MountableOptions{Base: NewMemory(0), Mounts: []MountConfig{{Point: "/db", FS: lower}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteFile(mountable, "/db/from-mount.txt", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := lower.Stat("from-mount.txt"); err != nil {
		t.Fatalf("mounted write did not reach the database: %v", err)
	}
}

// TestDatabaseDanglingSymlinkTargets keeps the behavior of fs.Memory for entry
// creation over a symbolic link that cannot be followed.
func TestDatabaseDanglingSymlinkTargets(t *testing.T) {
	memory := NewMemory(0)
	database := newSQLiteTestFS(t)
	for _, filesystem := range []iofs.FS{memory, database} {
		if err := Symlink(filesystem, "missing", "/link"); err != nil {
			t.Fatal(err)
		}
		if err := Mkdir(filesystem, "/link", 0o755); !errors.Is(err, iofs.ErrExist) {
			t.Fatalf("%T mkdir: %v", filesystem, err)
		}
		if err := CreateFile(filesystem, "/link", []byte("x"), 0o644); !errors.Is(err, iofs.ErrExist) {
			t.Fatalf("%T create: %v", filesystem, err)
		}
		if err := Symlink(filesystem, "other", "/link"); !errors.Is(err, iofs.ErrExist) {
			t.Fatalf("%T symlink: %v", filesystem, err)
		}
		if err := WriteFile(filesystem, "/link", []byte("written"), 0o644); err != nil {
			t.Fatalf("%T write: %v", filesystem, err)
		}
		got, err := ReadFile(filesystem, "/link")
		if err != nil || string(got) != "written" {
			t.Fatalf("%T read: %q %v", filesystem, got, err)
		}
	}
}
