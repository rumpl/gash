package text

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/rumpl/gash/internal/command"
	gfs "github.com/rumpl/gash/pkg/fs"
)

func TestSedBasicSubstituteAndPrint(t *testing.T) {
	assertCommand(t, commandSed, []string{"s/foo/bar/"}, "foo\nfood\n", "bar\nbard\n", nil)
	assertCommand(t, commandSed, []string{"-n", "s/foo/bar/p"}, "foo\nno\n", "bar\n", nil)
	assertCommand(t, commandSed, []string{"-n", "/é/p"}, "cafe\ncafé\n", "café\n", nil)
}

func TestSedDeletePrintAddresses(t *testing.T) {
	assertCommand(t, commandSed, []string{"2d"}, "one\ntwo\nthree\n", "one\nthree\n", nil)
	assertCommand(t, commandSed, []string{"-n", "2,3p"}, "one\ntwo\nthree\nfour\n", "two\nthree\n", nil)
	assertCommand(t, commandSed, []string{"-n", "$p"}, "one\ntwo\n", "two\n", nil)
	assertCommand(t, commandSed, []string{"-n", "1~2p"}, "one\ntwo\nthree\nfour\n", "one\nthree\n", nil)
	assertCommand(t, commandSed, []string{"/start/,/end/d"}, "a\nstart\nb\nend\nz\n", "a\nz\n", nil)
}

func TestSedMultipleScriptsAndFiles(t *testing.T) {
	files := map[string]string{"one.txt": "foo\n", "two.txt": "bar"}
	assertCommand(t, commandSed, []string{"-e", "s/foo/FOO/", "-e", "s/bar/BAR/", "one.txt", "two.txt"}, "", "FOO\nBAR", files)
	assertCommand(t, commandSed, []string{"-n", "-e", "p", "-", "one.txt"}, "stdin\n", "stdin\nfoo\n", files)
}

func TestSedScriptFileAndInPlace(t *testing.T) {
	fsys := gfs.NewMemory(0)
	_ = fsys.MkdirAll("work", 0o755)
	_ = fsys.WriteFile("work/script.sed", []byte("s/foo/bar/\n# ignored\n"), 0o644)
	_ = fsys.WriteFile("work/input.txt", []byte("foo\n"), 0o644)
	var out, stderr bytes.Buffer
	cwd := "/work"
	ctx := &command.Context{FS: fsys, Cwd: &cwd, Env: map[string]string{}, Stdin: strings.NewReader(""), Stdout: &out, Stderr: &stderr}
	if code := commandSed(context.Background(), []string{"-i", "-f", "script.sed", "input.txt"}, ctx); code != 0 {
		t.Fatalf("exit=%d stderr=%q", code, stderr.String())
	}
	data, _ := gfs.ReadFile(fsys, "/work/input.txt")
	if string(data) != "bar\n" {
		t.Fatalf("in-place content=%q", data)
	}
	if out.String() != "" {
		t.Fatalf("stdout=%q", out.String())
	}
}

func TestSedAdvancedCommands(t *testing.T) {
	assertCommand(t, commandSed, []string{"y/abc/ABC/"}, "abc xyz\n", "ABC xyz\n", nil)
	assertCommand(t, commandSed, []string{"-n", "N;P;D"}, "a\nb\nc\n", "a\nb\n", nil)
	assertCommand(t, commandSed, []string{"h;g"}, "x\n", "x\n", nil)
	assertCommand(t, commandSed, []string{"1a\\ after"}, "one\ntwo\n", "one\nafter\ntwo\n", nil)
	assertCommand(t, commandSed, []string{"2i\\ before"}, "one\ntwo\n", "one\nbefore\ntwo\n", nil)
	assertCommand(t, commandSed, []string{"2c\\ changed"}, "one\ntwo\n", "one\nchanged\n", nil)
	assertCommand(t, commandSed, []string{"-n", "="}, "a\nb\n", "1\n2\n", nil)
	assertCommand(t, commandSed, []string{"-n", "l"}, "a\t\\\n", "a\\t\\\\$\n", nil)
}

func TestSedRegexAndReplacement(t *testing.T) {
	assertCommand(t, commandSed, []string{"s/[[:digit:]]/N/g"}, "a1b2\n", "aNbN\n", nil)
	assertCommand(t, commandSed, []string{"s/\\(foo\\)/[\\1]/"}, "foo\n", "[foo]\n", nil)
	assertCommand(t, commandSed, []string{"-E", "s/(foo)+/X/"}, "foofoo\n", "X\n", nil)
	assertCommand(t, commandSed, []string{"s/foo/X/2"}, "foo foo foo\n", "foo X foo\n", nil)
	assertCommand(t, commandSed, []string{"s/foo/X/I"}, "FOO\n", "X\n", nil)
}

func TestSedFileReadWriteAndSecurity(t *testing.T) {
	files := map[string]string{"extra.txt": "extra\n", "input.txt": "one\n"}
	assertCommand(t, commandSed, []string{"1r extra.txt", "input.txt"}, "", "one\nextra\n", files)

	fsys := gfs.NewMemory(0)
	_ = fsys.MkdirAll("work", 0o755)
	_ = fsys.WriteFile("work/input.txt", []byte("one\n"), 0o644)
	var out, stderr bytes.Buffer
	cwd := "/work"
	ctx := &command.Context{FS: fsys, Cwd: &cwd, Env: map[string]string{}, Stdin: strings.NewReader("unsafe\n"), Stdout: &out, Stderr: &stderr}
	if code := commandSed(context.Background(), []string{"-n", "w out.txt", "input.txt"}, ctx); code != 0 {
		t.Fatalf("exit=%d stderr=%q", code, stderr.String())
	}
	data, _ := gfs.ReadFile(fsys, "/work/out.txt")
	if string(data) != "one\n" {
		t.Fatalf("write file content=%q", data)
	}

	out.Reset()
	stderr.Reset()
	code := commandSed(context.Background(), []string{"e"}, ctx)
	if code == 0 || !strings.Contains(stderr.String(), "shell execution") {
		t.Fatalf("security code=%d stderr=%q", code, stderr.String())
	}
}

func TestSedErrors(t *testing.T) {
	fsys := gfs.NewMemory(0)
	_ = fsys.MkdirAll("work", 0o755)
	var out, stderr bytes.Buffer
	cwd := "/work"
	ctx := &command.Context{FS: fsys, Cwd: &cwd, Env: map[string]string{}, Stdin: strings.NewReader(""), Stdout: &out, Stderr: &stderr}
	if code := commandSed(context.Background(), nil, ctx); code == 0 || !strings.Contains(stderr.String(), "no script specified") {
		t.Fatalf("missing script code=%d stderr=%q", code, stderr.String())
	}
	stderr.Reset()
	if code := commandSed(context.Background(), []string{"s/foo"}, ctx); code == 0 || !strings.Contains(stderr.String(), "unterminated") {
		t.Fatalf("bad script code=%d stderr=%q", code, stderr.String())
	}
	stderr.Reset()
	if code := commandSed(context.Background(), []string{"b nope"}, ctx); code == 0 || !strings.Contains(stderr.String(), "undefined label") {
		t.Fatalf("bad label code=%d stderr=%q", code, stderr.String())
	}
	stderr.Reset()
	if code := commandSed(context.Background(), []string{"-f", "missing.sed"}, ctx); code == 0 || !strings.Contains(stderr.String(), "couldn't open file") {
		t.Fatalf("missing script file code=%d stderr=%q", code, stderr.String())
	}
}

func TestSedBinaryAndNoTrailingNewline(t *testing.T) {
	assertCommand(t, commandSed, []string{"s/\\x00/Z/"}, "a\x00b\n", "aZb\n", nil)
	assertCommand(t, commandSed, []string{"s/a/A/"}, "abc", "Abc", nil)
}
