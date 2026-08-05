package files

import (
	"bytes"
	"io/fs"
	"strings"
	"testing"

	gfs "github.com/rumpl/gash/pkg/fs"
)

func TestInstallCopiesOverwritesAndSetsDefaultMode(t *testing.T) {
	filesystem := gfs.NewMemory(0)
	mustMkdir(t, filesystem, "work")
	if err := filesystem.WriteFile("work/src", []byte{'a', 0, 'b'}, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := filesystem.WriteFile("work/dest", []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	result := runCommandWithFS(t, commandInstall, []string{"src", "dest"}, filesystem)
	got, err := filesystem.ReadFile("work/dest")
	if err != nil {
		t.Fatal(err)
	}
	if result.exitCode != 0 || !bytes.Equal(got, []byte{'a', 0, 'b'}) || mode(filesystem, "work/dest") != 0o755 {
		t.Fatalf("exit=%d contents=%v mode=%04o stderr=%q", result.exitCode, got, mode(filesystem, "work/dest"), result.stderr)
	}
}

func TestInstallMultipleSourcesAndPartialFailure(t *testing.T) {
	filesystem := gfs.NewMemory(0)
	mustMkdir(t, filesystem, "work")
	mustMkdir(t, filesystem, "work/out")
	if err := filesystem.WriteFile("work/one", []byte("1"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := filesystem.WriteFile("work/two", []byte("2"), 0o644); err != nil {
		t.Fatal(err)
	}

	result := runCommandWithFS(t, commandInstall, []string{"one", "missing", "two", "out"}, filesystem)
	if result.exitCode == 0 || !exists(filesystem, "work/out/one") || !exists(filesystem, "work/out/two") || !strings.Contains(result.stderr, "missing") {
		t.Fatalf("exit=%d stderr=%q", result.exitCode, result.stderr)
	}
}

func TestInstallCreatesParentsDirectoriesAndModes(t *testing.T) {
	t.Run("D", func(t *testing.T) {
		result := runCommand(t, commandInstall, []string{"-D", "-m", "0644", "src", "a/b/dest"}, map[string]string{"src": "data"})
		if result.exitCode != 0 || !exists(result.filesystem, "work/a/b/dest") || mode(result.filesystem, "work/a/b/dest") != 0o644 {
			t.Fatalf("exit=%d mode=%04o stderr=%q", result.exitCode, mode(result.filesystem, "work/a/b/dest"), result.stderr)
		}
	})
	t.Run("directory", func(t *testing.T) {
		result := runCommand(t, commandInstall, []string{"--directory", "--mode=0700", "a/b", "c"}, nil)
		if result.exitCode != 0 || mode(result.filesystem, "work/a/b") != 0o700 || mode(result.filesystem, "work/c") != 0o700 {
			t.Fatalf("exit=%d modes=%04o,%04o stderr=%q", result.exitCode, mode(result.filesystem, "work/a/b"), mode(result.filesystem, "work/c"), result.stderr)
		}
	})
}

func TestInstallErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "missing operands", args: nil, want: "missing destination"},
		{name: "missing source", args: []string{"missing", "dest"}, want: "missing"},
		{name: "multiple target not directory", args: []string{"one", "two", "dest"}, want: "not a directory"},
		{name: "destination directory error", args: []string{"-D", "one", "dest"}, want: "is a directory"},
		{name: "unsupported ownership", args: []string{"-o", "root", "one", "dest"}, want: "not supported"},
		{name: "unsupported long flag", args: []string{"--compare", "one", "dest"}, want: "unrecognized option"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			filesystem := gfs.NewMemory(0)
			mustMkdir(t, filesystem, "work")
			_ = filesystem.WriteFile("work/one", []byte("1"), 0o644)
			_ = filesystem.WriteFile("work/two", []byte("2"), 0o644)
			if test.name == "destination directory error" {
				mustMkdir(t, filesystem, "work/dest")
			}
			result := runCommandWithFS(t, commandInstall, test.args, filesystem)
			if result.exitCode == 0 || !strings.Contains(result.stderr, test.want) {
				t.Fatalf("exit=%d stderr=%q", result.exitCode, result.stderr)
			}
		})
	}
}

func TestInstallHonorsOptionTerminator(t *testing.T) {
	result := runCommand(t, commandInstall, []string{"--", "-source", "dest"}, map[string]string{"-source": "ok"})
	if result.exitCode != 0 || !exists(result.filesystem, "work/dest") {
		t.Fatalf("exit=%d stderr=%q", result.exitCode, result.stderr)
	}
}

func TestInstallReadOnlyFilesystemFails(t *testing.T) {
	filesystem := gfs.NewMemory(0)
	mustMkdir(t, filesystem, "work")
	if err := filesystem.WriteFile("work/src", []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	result := runCommandWithStandardFS(t, commandInstall, []string{"src", "dest"}, readOnlyTestFS{FS: filesystem})
	if result.exitCode == 0 || !strings.Contains(result.stderr, "read-only") {
		t.Fatalf("exit=%d stderr=%q", result.exitCode, result.stderr)
	}
}

func mustMkdir(t *testing.T, filesystem interface {
	MkdirAll(string, fs.FileMode) error
}, name string,
) {
	t.Helper()
	if err := filesystem.MkdirAll(name, 0o755); err != nil {
		t.Fatal(err)
	}
}
