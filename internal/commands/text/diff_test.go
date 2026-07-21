package text

import (
	"context"
	"strings"
	"testing"

	"github.com/rumpl/gash/internal/command"
)

func TestDiffBasicComparison(t *testing.T) {
	files := map[string]string{
		"a.txt": "line1\nline2\nline3\n",
		"b.txt": "line1\nline2\nline3\n",
	}
	assertCommand(t, commandDiff, []string{"a.txt", "b.txt"}, "", "", files)

	code, stdout, stderr, _ := runTextCommandBytes(t, commandDiff, []string{"a.txt", "c.txt"}, nil, map[string][]byte{
		"a.txt": []byte("hello\n"),
		"c.txt": []byte("world\n"),
	})
	if code != 1 || string(stderr) != "" {
		t.Fatalf("exit=%d stderr=%q", code, stderr)
	}
	out := string(stdout)
	for _, want := range []string{"--- a.txt", "+++ c.txt", "@@", "-hello", "+world"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout missing %q: %q", want, out)
		}
	}
}

func TestDiffBriefReportSameAndIgnoreCase(t *testing.T) {
	files := map[string]string{"a.txt": "aaa\n", "b.txt": "bbb\n", "same.txt": "aaa\n", "case.txt": "AAA\n"}
	code, stdout, stderr, _ := runTextCommandBytes(t, commandDiff, []string{"-q", "a.txt", "b.txt"}, nil, stringFiles(files))
	if code != 1 || string(stdout) != "Files a.txt and b.txt differ\n" || stderr != "" {
		t.Fatalf("brief exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	code, stdout, stderr, _ = runTextCommandBytes(t, commandDiff, []string{"--brief", "a.txt", "same.txt"}, nil, stringFiles(files))
	if code != 0 || string(stdout) != "" || stderr != "" {
		t.Fatalf("brief same exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	code, stdout, stderr, _ = runTextCommandBytes(t, commandDiff, []string{"-s", "a.txt", "same.txt"}, nil, stringFiles(files))
	if code != 0 || string(stdout) != "Files a.txt and same.txt are identical\n" || stderr != "" {
		t.Fatalf("same exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	code, stdout, stderr, _ = runTextCommandBytes(t, commandDiff, []string{"--ignore-case", "a.txt", "case.txt"}, nil, stringFiles(files))
	if code != 0 || string(stdout) != "" || stderr != "" {
		t.Fatalf("ignore case exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

func TestDiffStdinAndUTF8(t *testing.T) {
	code, stdout, stderr, _ := runTextCommandBytes(t, commandDiff, []string{"-", "b.txt"}, []byte("from stdin\n"), map[string][]byte{"b.txt": []byte("from file\n")})
	if code != 1 || stderr != "" {
		t.Fatalf("stdin first exit=%d stderr=%q", code, stderr)
	}
	out := string(stdout)
	if !strings.Contains(out, "-from stdin") || !strings.Contains(out, "+from file") {
		t.Fatalf("stdin first stdout=%q", out)
	}

	code, stdout, stderr, _ = runTextCommandBytes(t, commandDiff, []string{"a.txt", "-"}, []byte("different\nshared\n"), map[string][]byte{"a.txt": []byte("한글\nshared\n")})
	if code != 1 || stderr != "" {
		t.Fatalf("utf8 stdin exit=%d stderr=%q", code, stderr)
	}
	out = string(stdout)
	if !strings.Contains(out, "한글") || !strings.Contains(out, "different") {
		t.Fatalf("utf8 stdout=%q", out)
	}
}

func TestDiffErrors(t *testing.T) {
	code, stdout, stderr, _ := runTextCommandBytes(t, commandDiff, []string{"missing.txt", "exists.txt"}, nil, map[string][]byte{"exists.txt": []byte("content\n")})
	if code != 2 || string(stdout) != "" || stderr != "diff: missing.txt: No such file or directory\n" {
		t.Fatalf("missing exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	code, stdout, stderr, _ = runTextCommandBytes(t, commandDiff, []string{"exists.txt"}, nil, map[string][]byte{"exists.txt": []byte("content\n")})
	if code != 2 || string(stdout) != "" || !strings.Contains(stderr, "missing operand") {
		t.Fatalf("operand exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	code, stdout, stderr, _ = runTextCommandBytes(t, commandDiff, []string{"a.txt", "b.txt", "extra.txt"}, nil, nil)
	if code != 2 || string(stdout) != "" || !strings.Contains(stderr, "missing operand") {
		t.Fatalf("extra operand exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	code, stdout, stderr, _ = runTextCommandBytes(t, commandDiff, []string{"--unknown", "a.txt", "b.txt"}, nil, nil)
	if code != 1 || string(stdout) != "" || !strings.Contains(stderr, "unrecognized option") {
		t.Fatalf("unknown long exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	code, stdout, stderr, _ = runTextCommandBytes(t, commandDiff, []string{"-z", "a.txt", "b.txt"}, nil, nil)
	if code != 1 || string(stdout) != "" || !strings.Contains(stderr, "invalid option") {
		t.Fatalf("unknown short exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	code, stdout, stderr, _ = runTextCommandBytes(t, commandDiff, []string{"dir", "exists.txt"}, nil, map[string][]byte{"dir/file.txt": []byte("nested\n"), "exists.txt": []byte("content\n")})
	if code != 2 || string(stdout) != "" || stderr != "diff: dir: No such file or directory\n" {
		t.Fatalf("dir exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

func TestDiffBinaryAndFinalNewline(t *testing.T) {
	assertCommandBytes(t, commandDiff, []string{"a.bin", "b.bin"}, nil, nil, map[string][]byte{
		"a.bin": {0x80, 0x90, 0xa0, 0xb0, 0xff},
		"b.bin": {0x80, 0x90, 0xa0, 0xb0, 0xff},
	})
	code, stdout, stderr, _ := runTextCommandBytes(t, commandDiff, []string{"a.bin", "b.bin"}, nil, map[string][]byte{
		"a.bin": {'A', 0, 'B'},
		"b.bin": {'A', 0, 'C'},
	})
	if code != 1 || string(stderr) != "" || len(stdout) == 0 {
		t.Fatalf("binary diff exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	code, stdout, stderr, _ = runTextCommandBytes(t, commandDiff, []string{"a.txt", "b.txt"}, nil, map[string][]byte{
		"a.txt": []byte("same"),
		"b.txt": []byte("same\n"),
	})
	if code != 1 || stderr != "" || !strings.Contains(string(stdout), "No newline at end of file") {
		t.Fatalf("newline diff exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

func TestDiffHelpIsRegistered(t *testing.T) {
	code, stdout, stderr, _ := runTextCommandBytes(t, func(ctx context.Context, args []string, c *command.Context) int {
		return commandDiff(ctx, args, c)
	}, []string{"--help"}, nil, nil)
	if code != 0 || !strings.Contains(string(stdout), "compare files line by line") || stderr != "" {
		t.Fatalf("help exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

func stringFiles(files map[string]string) map[string][]byte {
	out := map[string][]byte{}
	for name, content := range files {
		out[name] = []byte(content)
	}
	return out
}
