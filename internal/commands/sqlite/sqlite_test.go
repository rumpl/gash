package sqlite

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/rumpl/gash/internal/command"
	gfs "github.com/rumpl/gash/pkg/fs"
)

func runSQLite(t *testing.T, args []string, stdin string, files map[string]string) (int, string, string, *gfs.Memory) {
	t.Helper()
	fsys := gfs.NewMemory(0)
	if err := fsys.MkdirAll("work", 0o755); err != nil {
		t.Fatal(err)
	}
	for name, data := range files {
		if err := gfs.MkdirAll(fsys, "/work/"+dir(name), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := gfs.WriteFile(fsys, "/work/"+name, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cwd := "/work"
	var stdout, stderr bytes.Buffer
	ctx := &command.Context{FS: fsys, Cwd: &cwd, Env: map[string]string{}, Stdin: strings.NewReader(stdin), Stdout: &stdout, Stderr: &stderr}
	code := commandSQLite3(context.Background(), args, ctx)
	return code, stdout.String(), stderr.String(), fsys
}

func dir(name string) string {
	if i := strings.LastIndexByte(name, '/'); i >= 0 {
		return name[:i]
	}
	return "."
}

func TestSQLiteMemoryListAndHeaders(t *testing.T) {
	code, stdout, stderr, _ := runSQLite(t, []string{"-header", ":memory:", "CREATE TABLE t(id INTEGER, name TEXT); INSERT INTO t VALUES(1,'Ada'),(2,NULL); SELECT * FROM t ORDER BY id;"}, "", nil)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%q", code, stderr)
	}
	want := "id|name\n1|Ada\n2|\n"
	if stdout != want {
		t.Fatalf("stdout=%q want %q", stdout, want)
	}
}

func TestSQLiteOutputModes(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"-csv", "-header", ":memory:", "SELECT 1 AS a, 'x,y' AS b"}, "a,b\n1,\"x,y\"\n"},
		{[]string{"-json", ":memory:", "SELECT 1 AS a, 'x' AS b"}, "[{\"a\":1,\"b\":\"x\"}]\n"},
		{[]string{"-line", ":memory:", "SELECT 1 AS a, 'x' AS b"}, "    a = 1\n    b = x\n"},
		{[]string{"-tabs", "-header", ":memory:", "SELECT 1 AS a, 'x' AS b"}, "a\tb\n1\tx\n"},
		{[]string{"-quote", ":memory:", "SELECT 'can''t' AS q, NULL AS n"}, "'can''t',NULL\n"},
		{[]string{"-html", "-header", ":memory:", "SELECT '<x>' AS h"}, "<TR><TH>h</TH>\n</TR>\n<TR><TD>&lt;x&gt;</TD>\n</TR>\n"},
	}
	for _, tc := range cases {
		code, stdout, stderr, _ := runSQLite(t, tc.args, "", nil)
		if code != 0 {
			t.Fatalf("%v exit=%d stderr=%q", tc.args, code, stderr)
		}
		if stdout != tc.want {
			t.Fatalf("%v stdout=%q want %q", tc.args, stdout, tc.want)
		}
	}
}

func TestSQLiteWriteBackAndReadonly(t *testing.T) {
	code, _, stderr, fsys := runSQLite(t, []string{"data.db", "CREATE TABLE t(x); INSERT INTO t VALUES(7);"}, "", nil)
	if code != 0 {
		t.Fatalf("create exit=%d stderr=%q", code, stderr)
	}
	cwd := "/work"
	var stdout, errout bytes.Buffer
	ctx := &command.Context{FS: fsys, Cwd: &cwd, Env: map[string]string{}, Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &errout}
	if got := commandSQLite3(context.Background(), []string{"data.db", "SELECT x FROM t"}, ctx); got != 0 {
		t.Fatalf("select exit=%d stderr=%q", got, errout.String())
	}
	if stdout.String() != "7\n" {
		t.Fatalf("select stdout=%q", stdout.String())
	}

	if got := commandSQLite3(context.Background(), []string{"-readonly", "data.db", "INSERT INTO t VALUES(9);"}, ctx); got != 0 {
		t.Fatalf("readonly insert exit=%d stderr=%q", got, errout.String())
	}
	stdout.Reset()
	errout.Reset()
	if got := commandSQLite3(context.Background(), []string{"data.db", "SELECT x FROM t ORDER BY x"}, ctx); got != 0 {
		t.Fatalf("select2 exit=%d stderr=%q", got, errout.String())
	}
	if stdout.String() != "7\n" {
		t.Fatalf("readonly should not write back, stdout=%q", stdout.String())
	}
}

func TestSQLiteStdinCmdEchoOptionsAndErrors(t *testing.T) {
	code, stdout, stderr, _ := runSQLite(t, []string{"-echo", "-separator", ",", "-nullvalue", "NULL", "-cmd", "CREATE TABLE t(x)", ":memory:"}, "INSERT INTO t VALUES(NULL); SELECT x FROM t;", nil)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, "CREATE TABLE t(x); INSERT INTO") || !strings.HasSuffix(stdout, "NULL\n") {
		t.Fatalf("stdout=%q", stdout)
	}

	code, stdout, stderr, _ = runSQLite(t, []string{":memory:", "SELECT * FROM missing; SELECT 1;"}, "", nil)
	if code != 0 || !strings.Contains(stdout, "Error:") || !strings.HasSuffix(stdout, "1\n") || stderr != "" {
		t.Fatalf("non-bail code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	code, stdout, stderr, _ = runSQLite(t, []string{"-bail", ":memory:", "SELECT * FROM missing; SELECT 1;"}, "", nil)
	if code == 0 || stdout != "" || !strings.Contains(stderr, "Error:") {
		t.Fatalf("bail code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

func TestSQLiteRejectsHostEscapeSQL(t *testing.T) {
	for _, sql := range []string{"ATTACH DATABASE '/tmp/x' AS x", "VACUUM INTO '/tmp/x'", "SELECT load_extension('x')", ".open /tmp/x"} {
		code, stdout, stderr, _ := runSQLite(t, []string{":memory:", sql}, "", nil)
		if code != 0 || stdout == "" || !strings.Contains(stdout, "disabled") || stderr != "" {
			t.Fatalf("%q code=%d stdout=%q stderr=%q", sql, code, stdout, stderr)
		}
	}
}

func TestSQLiteVersion(t *testing.T) {
	code, stdout, stderr, _ := runSQLite(t, []string{"-version"}, "", nil)
	if code != 0 || strings.TrimSpace(stdout) == "" || stderr != "" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}
