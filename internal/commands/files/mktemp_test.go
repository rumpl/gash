package files

import (
	"bytes"
	iofs "io/fs"
	"strings"
	"testing"

	gfs "github.com/rumpl/gash/pkg/fs"
)

func TestMktempCreatesDefaultFileRelativeToCwd(t *testing.T) {
	withMktempRandom(t, bytes.NewReader(bytes.Repeat([]byte{0}, 10)))
	result := runCommand(t, commandMktemp, nil, nil)
	if result.exitCode != 0 || result.stderr != "" {
		t.Fatalf("exit=%d stderr=%q", result.exitCode, result.stderr)
	}
	if result.stdout != "tmp.aaaaaaaaaa\n" {
		t.Fatalf("stdout=%q", result.stdout)
	}
	if !exists(result.filesystem, "work/tmp.aaaaaaaaaa") {
		t.Fatal("default temporary file was not created in cwd")
	}
}

func TestMktempDirectoryAndExplicitTemplates(t *testing.T) {
	withMktempRandom(t, bytes.NewReader(bytes.Repeat([]byte{1}, 6)))
	result := runCommand(t, commandMktemp, []string{"-d", "cache.XXXXXX"}, nil)
	if result.exitCode != 0 || result.stdout != "cache.bbbbbb\n" {
		t.Fatalf("exit=%d stdout=%q stderr=%q", result.exitCode, result.stdout, result.stderr)
	}
	info, err := result.filesystem.Stat("work/cache.bbbbbb")
	if err != nil || !info.IsDir() {
		t.Fatalf("created entry is not a directory: info=%v err=%v", info, err)
	}
}

func TestMktempTmpdirForms(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{name: "short default", args: []string{"-p", "temps"}, want: "temps/tmp.aaaaaaaaaa\n"},
		{name: "long basename", args: []string{"--tmpdir=temps", "job.XXX"}, want: "temps/job.aaa\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withMktempRandom(t, bytes.NewReader(bytes.Repeat([]byte{0}, 10)))
			filesystem := gfs.NewMemory(0)
			if err := filesystem.MkdirAll("work/temps", 0o755); err != nil {
				t.Fatal(err)
			}
			result := runCommandWithFS(t, commandMktemp, tc.args, filesystem)
			if result.exitCode != 0 || result.stdout != tc.want {
				t.Fatalf("exit=%d stdout=%q stderr=%q", result.exitCode, result.stdout, result.stderr)
			}
			if !exists(filesystem, "work/"+strings.TrimSpace(tc.want)) {
				t.Fatal("temporary entry missing")
			}
		})
	}
}

func TestMktempRetriesCollisions(t *testing.T) {
	withMktempRandom(t, bytes.NewReader(append(bytes.Repeat([]byte{0}, 3), bytes.Repeat([]byte{1}, 3)...)))
	result := runCommand(t, commandMktemp, []string{"item.XXX"}, map[string]string{"item.aaa": "existing"})
	if result.exitCode != 0 || result.stdout != "item.bbb\n" {
		t.Fatalf("exit=%d stdout=%q stderr=%q", result.exitCode, result.stdout, result.stderr)
	}
	data, err := result.filesystem.ReadFile("work/item.aaa")
	if err != nil || string(data) != "existing" {
		t.Fatalf("collision was overwritten: data=%q err=%v", data, err)
	}
}

func TestMktempFailsAfterBoundedCollisionRetries(t *testing.T) {
	withMktempRandom(t, bytes.NewReader(bytes.Repeat([]byte{0}, mktempAttempts*3)))
	result := runCommand(t, commandMktemp, []string{"item.XXX"}, map[string]string{"item.aaa": "existing"})
	if result.exitCode == 0 || !strings.Contains(result.stderr, "after 100 attempts") {
		t.Fatalf("exit=%d stderr=%q", result.exitCode, result.stderr)
	}
}

func TestMktempReportsFilesystemAndArgumentErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
		fs   iofs.FS
		want string
	}{
		{name: "bad template", args: []string{"bad.XX"}, want: "at least 3"},
		{name: "missing parent", args: []string{"missing/file.XXX"}, want: "No such file or directory"},
		{name: "ambiguous tmpdir", args: []string{"-p", "temps", "nested/file.XXX"}, want: "must be a basename"},
		{name: "unsupported", args: []string{"-u"}, want: "invalid option"},
		{name: "readonly", args: []string{"file.XXX"}, fs: gfs.ReadOnly(gfs.NewMemory(0)), want: "read-only"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			withMktempRandom(t, bytes.NewReader(bytes.Repeat([]byte{0}, 10)))
			var result commandResult
			if tc.fs != nil {
				result = runCommandWithStandardFS(t, commandMktemp, tc.args, tc.fs)
			} else {
				result = runCommand(t, commandMktemp, tc.args, nil)
			}
			if result.exitCode == 0 || !strings.Contains(result.stderr, tc.want) {
				t.Fatalf("exit=%d stderr=%q, want substring %q", result.exitCode, result.stderr, tc.want)
			}
		})
	}
}

func TestMktempNormalizesRelativeTemplateOutput(t *testing.T) {
	tests := []struct {
		name     string
		template string
		want     string
		created  string
	}{
		{name: "current directory", template: "./local.XXX", want: "local.aaa\n", created: "work/local.aaa"},
		{name: "redundant components", template: "nested//./item.XXX", want: "nested/item.aaa\n", created: "work/nested/item.aaa"},
		{name: "parent traversal", template: "../root.XXX", want: "/root.aaa\n", created: "root.aaa"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			withMktempRandom(t, bytes.NewReader(bytes.Repeat([]byte{0}, 3)))
			filesystem := gfs.NewMemory(0)
			if err := filesystem.MkdirAll("work/nested", 0o755); err != nil {
				t.Fatal(err)
			}
			result := runCommandWithFS(t, commandMktemp, []string{tc.template}, filesystem)
			if result.exitCode != 0 || result.stdout != tc.want || result.stderr != "" {
				t.Fatalf("exit=%d stdout=%q stderr=%q", result.exitCode, result.stdout, result.stderr)
			}
			if !exists(filesystem, tc.created) {
				t.Fatalf("created entry %q is missing", tc.created)
			}
		})
	}
}

func TestMktempAbsoluteTemplatePrintsUsableAbsolutePath(t *testing.T) {
	withMktempRandom(t, bytes.NewReader(bytes.Repeat([]byte{0}, 3)))
	result := runCommand(t, commandMktemp, []string{"/root.XXX"}, nil)
	if result.exitCode != 0 || result.stdout != "/root.aaa\n" || !exists(result.filesystem, "root.aaa") {
		t.Fatalf("exit=%d stdout=%q stderr=%q", result.exitCode, result.stdout, result.stderr)
	}
}

func withMktempRandom(t *testing.T, reader *bytes.Reader) {
	t.Helper()
	old := mktempRandom
	mktempRandom = reader
	t.Cleanup(func() { mktempRandom = old })
}
