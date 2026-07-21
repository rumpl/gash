package text

import (
	"bytes"
	"context"
	"path"
	"strings"
	"testing"

	"github.com/rumpl/gash/internal/command"
	gfs "github.com/rumpl/gash/pkg/fs"
)

func TestGrep(t *testing.T) {
	assertCommand(t, commandGrep, []string{"needle"}, "hay\nneedle\n", "needle\n", nil)
}

func TestFGrepFixedStringMetacharacters(t *testing.T) {
	assertCommand(t, commandFGrep, []string{"a.c"}, "abc\na.c\naxc\n", "a.c\n", nil)
	assertCommand(t, commandFGrep, []string{"[x]"}, "x\n[x]\n", "[x]\n", nil)
}

func TestFGrepStdinDashAndUTF8Bytes(t *testing.T) {
	assertCommand(t, commandFGrep, []string{"é"}, "cafe\ncafé\n", "café\n", nil)
	assertCommand(t, commandFGrep, []string{"needle", "-"}, "needle\nother\n", "needle\n", nil)

	run := func(ctx context.Context, args []string, c *command.Context) int {
		return commandFGrep(ctx, args, c)
	}
	filesystem := gfs.NewMemory(0)
	_ = filesystem.MkdirAll("work", 0o755)
	var out, stderr bytes.Buffer
	cwd := "/work"
	ctx := &command.Context{FS: filesystem, Cwd: &cwd, Env: map[string]string{}, Stdin: bytes.NewReader([]byte{'a', 0xff, 'b', '\n'}), Stdout: &out, Stderr: &stderr}
	if code := run(context.Background(), []string{string([]byte{0xff})}, ctx); code != 0 {
		t.Fatalf("exit=%d stderr=%q", code, stderr.String())
	}
	if got := out.Bytes(); !bytes.Equal(got, []byte{'a', 0xff, 'b', '\n'}) {
		t.Fatalf("stdout bytes=%v", got)
	}
}

func TestFGrepMultipleFilesOutputAndFlags(t *testing.T) {
	files := map[string]string{
		"one.txt": "alpha\nneedle\n",
		"two.txt": "needle two\nnone\nneedle again\n",
	}
	assertCommand(t, commandFGrep, []string{"-n", "needle", "one.txt", "two.txt"}, "", "one.txt:2:needle\ntwo.txt:1:needle two\ntwo.txt:3:needle again\n", files)
	assertCommand(t, commandFGrep, []string{"-c", "needle", "one.txt", "two.txt"}, "", "one.txt:1\ntwo.txt:2\n", files)
	assertCommand(t, commandFGrep, []string{"-l", "needle", "one.txt", "two.txt"}, "", "one.txt\ntwo.txt\n", files)
	assertCommand(t, commandFGrep, []string{"-L", "absent", "one.txt", "two.txt"}, "", "one.txt\ntwo.txt\n", files)
	assertCommand(t, commandFGrep, []string{"-o", "needle", "two.txt"}, "", "needle\nneedle\n", files)
	assertCommand(t, commandFGrep, []string{"-m1", "needle", "two.txt"}, "", "needle two\n", files)
	assertCommand(t, commandFGrep, []string{"-h", "needle", "one.txt", "two.txt"}, "", "needle\nneedle two\nneedle again\n", files)
}

func TestFGrepCaseInvertWordLineAndContext(t *testing.T) {
	assertCommand(t, commandFGrep, []string{"-i", "needle"}, "Needle\nother\n", "Needle\n", nil)
	assertCommand(t, commandFGrep, []string{"-v", "needle"}, "needle\nother\n", "other\n", nil)
	assertCommand(t, commandFGrep, []string{"-w", "he"}, "he\nthe\nhe-man\n", "he\nhe-man\n", nil)
	assertCommand(t, commandFGrep, []string{"-x", "he"}, "he\nhey\n", "he\n", nil)
	assertCommand(t, commandFGrep, []string{"-A1", "needle"}, "before\nneedle\nafter\n", "needle\nafter\n", nil)
	assertCommand(t, commandFGrep, []string{"-B", "1", "needle"}, "before\nneedle\nafter\n", "before\nneedle\n", nil)
}

func TestFGrepRecursiveIncludeExclude(t *testing.T) {
	files := map[string]string{
		"a.txt":      "needle\n",
		"b.log":      "needle\n",
		"skip.txt":   "needle\n",
		"dir/c.txt":  "needle\n",
		"hide/d.txt": "needle\n",
	}
	assertCommand(t, commandFGrep, []string{"-r", "--include=*.txt", "--exclude=skip.txt", "--exclude-dir=hide", "needle", "."}, "", "a.txt:needle\ndir/c.txt:needle\n", files)
}

func TestFGrepStatusesAndErrors(t *testing.T) {
	if code, _, _ := runTextCommand(t, commandFGrep, []string{"needle"}, "none\n", nil); code != 1 {
		t.Fatalf("no match exit=%d, want 1", code)
	}
	if code, _, stderr := runTextCommand(t, commandFGrep, nil, "", nil); code != 2 || stderr != "grep: missing pattern\n" {
		t.Fatalf("missing pattern exit=%d stderr=%q", code, stderr)
	}
	if code, stdout, stderr := runTextCommand(t, commandFGrep, []string{"needle", "missing.txt"}, "", nil); code != 2 || stdout != "" || !strings.Contains(stderr, "grep: missing.txt: No such file or directory") {
		t.Fatalf("missing file exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if code, stdout, stderr := runTextCommand(t, commandFGrep, []string{"needle", "dir"}, "", map[string]string{"dir/file.txt": "needle\n"}); code != 1 || stdout != "" || stderr != "grep: dir: Is a directory\n" {
		t.Fatalf("directory exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if code, stdout, stderr := runTextCommand(t, commandFGrep, []string{"-q", "needle", "missing.txt", "ok.txt"}, "", map[string]string{"ok.txt": "needle\n"}); code != 0 || stdout != "" || !strings.Contains(stderr, "missing.txt") {
		t.Fatalf("quiet missing exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

func runTextCommand(t *testing.T, run commandFunc, args []string, stdin string, files map[string]string) (int, string, string) {
	t.Helper()
	filesystem := gfs.NewMemory(0)
	_ = filesystem.MkdirAll("work", 0o755)
	for name, content := range files {
		dir := path.Dir(name)
		if dir != "." {
			_ = filesystem.MkdirAll("work/"+dir, 0o755)
		}
		if err := filesystem.WriteFile("work/"+name, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var out, stderr bytes.Buffer
	cwd := "/work"
	ctx := &command.Context{FS: filesystem, Cwd: &cwd, Env: map[string]string{}, Stdin: bytes.NewBufferString(stdin), Stdout: &out, Stderr: &stderr}
	code := run(context.Background(), args, ctx)
	return code, out.String(), stderr.String()
}
