package commands

import (
	"bytes"
	"context"
	"path"
	"testing"

	"github.com/rumpl/gash/internal/command"
	gfs "github.com/rumpl/gash/pkg/fs"
)

type commandFunc = command.Func

func runCommand(t *testing.T, run commandFunc, args []string, stdin []byte, files map[string][]byte) (int, []byte, string, *gfs.Memory) {
	t.Helper()
	filesystem := gfs.NewMemory(0)
	if err := filesystem.MkdirAll("work", 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range files {
		dir := path.Dir(name)
		if dir != "." {
			if err := filesystem.MkdirAll("work/"+dir, 0o755); err != nil {
				t.Fatal(err)
			}
		}
		if err := filesystem.WriteFile("work/"+name, content, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var stdout, stderr bytes.Buffer
	cwd := "/work"
	ctx := &command.Context{FS: filesystem, Cwd: &cwd, Env: map[string]string{}, Stdin: bytes.NewReader(stdin), Stdout: &stdout, Stderr: &stderr}
	code := run(context.Background(), args, ctx)
	return code, stdout.Bytes(), stderr.String(), filesystem
}

func assertCommandBytes(t *testing.T, run commandFunc, args []string, stdin []byte, wantStdout []byte, files map[string][]byte) {
	t.Helper()
	code, stdout, stderr, _ := runCommand(t, run, args, stdin, files)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%q", code, stderr)
	}
	if !bytes.Equal(stdout, wantStdout) {
		t.Fatalf("stdout=%q, want %q", stdout, wantStdout)
	}
}

func TestCatCommonWorkflows(t *testing.T) {
	assertCommandBytes(t, commandCat, nil, []byte("a\x00b\n"), []byte("a\x00b\n"), nil)
	assertCommandBytes(t, commandCat, []string{"one.txt", "-", "two.txt"}, []byte("stdin\n"), []byte("one\nstdin\ntwo\n"), map[string][]byte{
		"one.txt": []byte("one\n"),
		"two.txt": []byte("two\n"),
	})

	code, stdout, stderr, _ := runCommand(t, commandCat, []string{"missing.txt"}, nil, nil)
	if code == 0 || len(stdout) != 0 || stderr == "" {
		t.Fatalf("missing file exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

func TestBase64CommonWorkflows(t *testing.T) {
	assertCommandBytes(t, commandBase64, nil, []byte("hello"), []byte("aGVsbG8=\n"), nil)
	assertCommandBytes(t, commandBase64, []string{"--decode"}, []byte("YQBi/w==\n"), []byte{'a', 0, 'b', 0xff}, nil)
	assertCommandBytes(t, commandBase64, []string{"bin.dat"}, nil, []byte("AP8=\n"), map[string][]byte{"bin.dat": {0, 0xff}})
	assertCommandBytes(t, commandBase64, []string{"-w", "4"}, []byte("hello"), []byte("aGVs\nbG8=\n"), nil)

	code, stdout, stderr, _ := runCommand(t, commandBase64, []string{"--bogus"}, nil, nil)
	if code == 0 || len(stdout) != 0 || stderr == "" {
		t.Fatalf("invalid option exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

func TestChecksumCommonWorkflows(t *testing.T) {
	assertCommandBytes(t, checksum("md5"), nil, []byte("hello"), []byte("5d41402abc4b2a76b9719d911017c592  -\n"), nil)
	assertCommandBytes(t, checksum("sha1"), []string{"a.txt"}, nil, []byte("55ca6286e3e4f4fba5d0448333fa99fc5a404a73  a.txt\n"), map[string][]byte{"a.txt": []byte("hi\n")})
	assertCommandBytes(t, checksum("sha256"), []string{"a.txt", "b.txt"}, nil, []byte("98ea6e4f216f2fb4b69fff9b3a44842c38686ca685f3f55dc48c5d3fb1107be4  a.txt\nabc6fd595fc079d3114d4b71a4d84b1d1d0f79df1e70f8813212f2a65d8916df  b.txt\n"), map[string][]byte{
		"a.txt": []byte("hi\n"),
		"b.txt": []byte("bye\n"),
	})

	code, stdout, stderr, _ := runCommand(t, checksum("md5"), []string{"missing.txt"}, nil, nil)
	if code == 0 || len(stdout) != 0 || stderr == "" {
		t.Fatalf("missing file exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}
