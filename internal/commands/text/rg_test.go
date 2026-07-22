package text

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/rumpl/gash/internal/command"
	gfs "github.com/rumpl/gash/pkg/fs"
)

func TestRgBasicRecursiveSmartCaseAndFormatting(t *testing.T) {
	files := map[string]string{
		"a.txt":      "needle\nother\n",
		"dir/b.go":   "fmt.Println(\"needle\")\n",
		"dir/c.log":  "NEEDLE\n",
		".hidden.md": "needle\n",
	}
	assertCommand(t, commandRg, []string{"needle"}, "", "a.txt:1:needle\ndir/b.go:1:fmt.Println(\"needle\")\ndir/c.log:1:NEEDLE\n", files)
	code, stdout, stderr, _ := runTextCommandBytes(t, commandRg, []string{"Needle"}, nil, rgStringFiles(files))
	if code != 1 || len(stdout) != 0 || stderr != "" {
		t.Fatalf("smart-case no match exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	assertCommand(t, commandRg, []string{"-i", "Needle"}, "", "a.txt:1:needle\ndir/b.go:1:fmt.Println(\"needle\")\ndir/c.log:1:NEEDLE\n", files)
	assertCommand(t, commandRg, []string{"--hidden", "needle"}, "", ".hidden.md:1:needle\na.txt:1:needle\ndir/b.go:1:fmt.Println(\"needle\")\ndir/c.log:1:NEEDLE\n", files)
}

func rgStringFiles(files map[string]string) map[string][]byte {
	out := map[string][]byte{}
	for name, content := range files {
		out[name] = []byte(content)
	}
	return out
}

func TestRgStdinSingleFileLineNumbersAndNoFilename(t *testing.T) {
	assertCommand(t, commandRg, []string{"needle"}, "hay\nneedle\n", "2:needle\n", nil)
	assertCommand(t, commandRg, []string{"needle", "one.txt"}, "", "needle\n", map[string]string{"one.txt": "needle\n"})
	assertCommand(t, commandRg, []string{"-n", "needle", "one.txt"}, "", "1:needle\n", map[string]string{"one.txt": "needle\n"})
	assertCommand(t, commandRg, []string{"-I", "needle", "."}, "", "1:needle\n", map[string]string{"one.txt": "needle\n"})
}

func TestRgFixedOnlyMatchingCountAndMaxCount(t *testing.T) {
	files := map[string]string{"one.txt": "a.c abc a.c\nneedle\nneedle again\n"}
	assertCommand(t, commandRg, []string{"-F", "-o", "a.c", "one.txt"}, "", "a.c\na.c\n", files)
	assertCommand(t, commandRg, []string{"-c", "needle", "one.txt"}, "", "2\n", files)
	assertCommand(t, commandRg, []string{"--count-matches", "needle", "one.txt"}, "", "2\n", files)
	assertCommand(t, commandRg, []string{"-m1", "needle", "one.txt"}, "", "needle\n", files)
}

func TestRgFilteringGlobsTypesGitignoreAndFilesMode(t *testing.T) {
	files := map[string]string{
		"a.go":               "needle\n",
		"b.txt":              "needle\n",
		"skip.log":           "needle\n",
		"vendor/v.go":        "needle\n",
		"node_modules/n.txt": "needle\n",
		".gitignore":         "skip.log\nignored/\n!important.txt\n",
		"ignored/x.go":       "needle\n",
		"important.txt":      "needle\n",
	}
	assertCommand(t, commandRg, []string{"-t", "go", "needle"}, "", "a.go:1:needle\n", files)
	assertCommand(t, commandRg, []string{"-g", "*.txt", "needle"}, "", "b.txt:1:needle\nimportant.txt:1:needle\n", files)
	assertCommand(t, commandRg, []string{"--no-ignore", "needle"}, "", "a.go:1:needle\nb.txt:1:needle\nignored/x.go:1:needle\nimportant.txt:1:needle\nnode_modules/n.txt:1:needle\nskip.log:1:needle\nvendor/v.go:1:needle\n", files)
	assertCommand(t, commandRg, []string{"--files", "-g", "*.go"}, "", "a.go\n", files)
}

func TestRgContextFilesWithMatchesWithoutMatchAndNull(t *testing.T) {
	files := map[string]string{"one.txt": "before\nneedle\nafter\n", "two.txt": "none\n"}
	assertCommand(t, commandRg, []string{"-A1", "needle", "one.txt"}, "", "needle\nafter\n", files)
	assertCommand(t, commandRg, []string{"-l", "needle", "one.txt", "two.txt"}, "", "one.txt\n", files)
	assertCommand(t, commandRg, []string{"--files-without-match", "needle", "one.txt", "two.txt"}, "", "two.txt\n", files)
	assertCommandBytes(t, commandRg, []string{"-0", "-l", "needle", "one.txt"}, nil, []byte("one.txt\x00"), map[string][]byte{"one.txt": []byte("needle\n")})
}

func TestRgPatternFilesBinaryJSONAndErrors(t *testing.T) {
	assertCommand(t, commandRg, []string{"-f", "patterns.txt", "data.txt"}, "", "needle\n", map[string]string{"patterns.txt": "needle\n", "data.txt": "needle\n"})
	code, stdout, stderr, _ := runTextCommandBytes(t, commandRg, []string{"needle"}, nil, map[string][]byte{"bin.dat": []byte("abc\x00needle\n")})
	if code != 1 || len(stdout) != 0 || stderr != "" {
		t.Fatalf("binary skip exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	assertCommand(t, commandRg, []string{"-a", "needle"}, "", "bin.dat:1:abc\x00needle\n", map[string]string{"bin.dat": "abc\x00needle\n"})

	code, stdout, stderr, _ = runTextCommandBytes(t, commandRg, []string{"--json", "needle", "one.txt"}, nil, map[string][]byte{"one.txt": []byte("needle\n")})
	if code != 0 || stderr != "" || !strings.Contains(string(stdout), `"type":"match"`) || !strings.Contains(string(stdout), `"type":"summary"`) {
		t.Fatalf("json exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	code, stdout, stderr, _ = runTextCommandBytes(t, commandRg, nil, nil, nil)
	if code != 2 || string(stdout) != "" || stderr != "rg: no pattern given\n" {
		t.Fatalf("missing pattern exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	code, _, stderr, _ = runTextCommandBytes(t, commandRg, []string{"-P", "needle"}, nil, nil)
	if code != 1 || !strings.Contains(stderr, "PCRE2 is not supported") {
		t.Fatalf("pcre exit=%d stderr=%q", code, stderr)
	}
}

func TestRgSymlinkLoopSkippedByDefault(t *testing.T) {
	fsys := gfs.NewMemory(0)
	_ = fsys.MkdirAll("work/dir", 0o755)
	_ = fsys.WriteFile("work/dir/a.txt", []byte("needle\n"), 0o644)
	_ = fsys.Symlink("../dir", "work/dir/loop")
	var out, stderr bytes.Buffer
	cwd := "/work"
	ctx := &command.Context{FS: fsys, Cwd: &cwd, Env: map[string]string{}, Stdin: bytes.NewReader(nil), Stdout: &out, Stderr: &stderr}
	if code := commandRg(context.Background(), []string{"needle"}, ctx); code != 0 || out.String() != "dir/a.txt:1:needle\n" || stderr.String() != "" {
		t.Fatalf("exit stdout stderr = %d %q %q", code, out.String(), stderr.String())
	}
}
