package archive_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/rumpl/gash/internal/gashtest"
	gfs "github.com/rumpl/gash/pkg/fs"
	"github.com/rumpl/gash/pkg/gash"
)

const testMaxArchiveExpandedBytes = 512 << 20

func TestGzipGunzipZcatCompatibility(t *testing.T) {
	cases := []gashtest.Case{
		{
			Name:       "gzip stdin to stdout handles binary",
			Script:     "gzip -c | gunzip -c",
			Stdin:      "hello\x00world\n",
			WantStdout: "hello\x00world\n",
		},
		{
			Name:       "gzip file writes gz file and gunzip restores",
			Files:      map[string]string{"home/user/data.txt": "payload"},
			Script:     "gzip data.txt && zcat data.txt.gz && gunzip data.txt.gz",
			WantStdout: "payload",
			WantFiles:  map[string]string{"/home/user/data.txt": "payload"},
			Check: func(t testing.TB, shell *gash.Bash, _ gash.Result) {
				t.Helper()
				compressed, err := gfs.ReadFile(shell.FS, "/home/user/data.txt.gz")
				if err != nil {
					t.Fatalf("read gzip: %v", err)
				}
				zr, err := gzip.NewReader(bytes.NewReader(compressed))
				if err != nil {
					t.Fatalf("new gzip reader: %v", err)
				}
				got, err := io.ReadAll(zr)
				if err != nil {
					t.Fatalf("read gzip payload: %v", err)
				}
				if string(got) != "payload" {
					t.Fatalf("gzip payload = %q", got)
				}
			},
		},
		{
			Name:       "malformed gzip fails",
			Script:     "printf not-gzip-data | gunzip -c",
			WantStatus: 1,
			WantStderr: "gzip: -: gzip: invalid header\n",
		},
	}
	gashtest.RunAll(t, cases)
}

func TestTarCreateListExtractCompatibility(t *testing.T) {
	gashtest.Run(t, gashtest.Case{
		Name: "create list and extract gzip tar",
		Files: map[string]string{
			"home/user/src/a.txt":       "alpha",
			"home/user/src/dir/b.txt":   "bravo",
			"home/user/src/dir/.hidden": "dot",
		},
		Script:     "tar -czf bundle.tgz src && tar -tzf bundle.tgz && mkdir out && tar -xzf bundle.tgz -C out",
		WantStdout: "src/\nsrc/a.txt\nsrc/dir/\nsrc/dir/.hidden\nsrc/dir/b.txt\n",
		WantFiles: map[string]string{
			"/home/user/out/src/a.txt":       "alpha",
			"/home/user/out/src/dir/b.txt":   "bravo",
			"/home/user/out/src/dir/.hidden": "dot",
		},
	})
}

func TestTarExtractRejectsHostilePaths(t *testing.T) {
	cases := []struct {
		name   string
		header *tar.Header
		body   string
		stderr string
	}{
		{
			name:   "traversal",
			header: &tar.Header{Name: "../escape.txt", Mode: 0o644, Size: 4},
			body:   "boom",
			stderr: "tar: refusing unsafe archive path \"../escape.txt\"\n",
		},
		{
			name:   "absolute",
			header: &tar.Header{Name: "/tmp/escape.txt", Mode: 0o644, Size: 4},
			body:   "boom",
			stderr: "tar: refusing unsafe archive path \"/tmp/escape.txt\"\n",
		},
		{
			name:   "symlink escape",
			header: &tar.Header{Name: "link", Typeflag: tar.TypeSymlink, Linkname: "../../escape", Mode: 0o777},
			stderr: "tar: refusing unsafe symlink \"link\" -> \"../../escape\"\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			archive := makeTar(t, tc.header, tc.body, false)
			shell, err := gash.New(gash.Options{})
			if err != nil {
				t.Fatal(err)
			}
			res := shell.Exec(context.Background(), "tar -xf -", gash.ExecOptions{Stdin: string(archive)})
			if res.ExitCode != 1 {
				t.Fatalf("exit=%d stdout=%q stderr=%q", res.ExitCode, res.Stdout, res.Stderr)
			}
			if res.Stderr != tc.stderr {
				t.Fatalf("stderr=%q want %q", res.Stderr, tc.stderr)
			}
			if _, err := gfs.Stat(shell.FS, "/home/escape.txt"); err == nil {
				t.Fatal("escape file was created")
			}
		})
	}
}

func TestTarRejectsCompressedBombByLimit(t *testing.T) {
	archive := makeTar(t, &tar.Header{Name: "big.bin", Mode: 0o644, Size: int64(testMaxArchiveExpandedBytes + 1)}, strings.Repeat("x", testMaxArchiveExpandedBytes+1), true)
	shell, err := gash.New(gash.Options{})
	if err != nil {
		t.Fatal(err)
	}
	res := shell.Exec(context.Background(), "tar -xzf -", gash.ExecOptions{Stdin: string(archive)})
	if res.ExitCode != 1 {
		t.Fatalf("exit=%d stdout=%q stderr=%q", res.ExitCode, res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stderr, "archive expanded output limit exceeded") {
		t.Fatalf("stderr=%q", res.Stderr)
	}
}

func makeTar(t *testing.T, hdr *tar.Header, body string, compress bool) []byte {
	t.Helper()
	var tarBuf bytes.Buffer
	tw := tar.NewWriter(&tarBuf)
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("write header: %v", err)
	}
	if hdr.Size > 0 {
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatalf("write body: %v", err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if !compress {
		return tarBuf.Bytes()
	}
	var gz bytes.Buffer
	zw := gzip.NewWriter(&gz)
	if _, err := zw.Write(tarBuf.Bytes()); err != nil {
		t.Fatalf("write gzip: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	return gz.Bytes()
}
