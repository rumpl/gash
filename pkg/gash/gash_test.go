package gash

import (
	"context"
	"io"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

func newTestBash(t *testing.T) *Bash {
	t.Helper()
	b, e := New(Options{})
	if e != nil {
		t.Fatal(e)
	}
	return b
}

func TestExecFilesystemPersistsShellStateDoesNot(t *testing.T) {
	b := newTestBash(t)
	r := b.Exec(context.Background(), `echo "Hello $NAME" > greeting.txt`, ExecOptions{Env: map[string]string{"NAME": "Alice"}})
	if r.ExitCode != 0 {
		t.Fatalf("%+v", r)
	}
	r = b.Exec(context.Background(), "cat greeting.txt", ExecOptions{})
	if r.Stdout != "Hello Alice\n" {
		t.Fatalf("stdout=%q", r.Stdout)
	}
	r = b.Exec(context.Background(), "cd /tmp; pwd", ExecOptions{})
	if r.Stdout != "/tmp\n" {
		t.Fatalf("stdout=%q", r.Stdout)
	}
	if b.GetCwd() != "/home/user" {
		t.Fatalf("cwd leaked: %s", b.GetCwd())
	}
}

func TestPipelinesRedirectionsAndConditionals(t *testing.T) {
	b := newTestBash(t)
	r := b.Exec(context.Background(), "printf 'pear\\napple\\npear\\n' | sort | uniq | grep apple && echo found || echo missing", ExecOptions{})
	if r.ExitCode != 0 || r.Stdout != "apple\nfound\n" {
		t.Fatalf("%+v", r)
	}
	r = b.Exec(context.Background(), "echo one > x; echo two >> x; wc -l < x", ExecOptions{})
	if strings.TrimSpace(r.Stdout) != "2" {
		t.Fatalf("%+v", r)
	}
}

func TestBashLanguageFeatures(t *testing.T) {
	b := newTestBash(t)
	script := `
		greet() { printf 'hello %s\n' "$1"; }
		total=0
		for n in 1 2 3; do total=$((total + n)); done
		if [[ $total -eq 6 ]]; then greet "$(echo world)"; fi
		cat <<'EOF'
heredoc $total
EOF
	`
	result := b.Exec(context.Background(), script, ExecOptions{})
	if result.ExitCode != 0 || result.Stdout != "hello world\nheredoc $total\n" {
		t.Fatalf("%+v", result)
	}
}

func TestVirtualIdentityAndNoHostFilesystem(t *testing.T) {
	b := newTestBash(t)
	result := b.Exec(context.Background(), `printf '%s:%s:%s' "$UID" "$EUID" "$GID"`, ExecOptions{})
	if result.Stdout != "1000:1000:1000" {
		t.Fatalf("host identity leaked: %+v", result)
	}
	result = b.Exec(context.Background(), "cat /etc/passwd", ExecOptions{})
	if result.ExitCode == 0 {
		t.Fatalf("host filesystem was visible: %+v", result)
	}
}

func TestCustomCommand(t *testing.T) {
	upper := Command{Name: "upper", Run: func(_ context.Context, _ []string, c *CommandContext) int {
		d, _ := io.ReadAll(c.Stdin)
		c.Stdout.Write([]byte(strings.ToUpper(string(d))))
		return 0
	}}
	b, e := New(Options{Commands: []Command{upper}})
	if e != nil {
		t.Fatal(e)
	}
	r := b.Exec(context.Background(), "echo hello | upper", ExecOptions{})
	if r.Stdout != "HELLO\n" {
		t.Fatalf("%+v", r)
	}
}

func TestStandardReadOnlyFilesystem(t *testing.T) {
	b, err := New(Options{FS: fstest.MapFS{"data/message.txt": {Data: []byte("from mapfs\n")}}, Cwd: "/"})
	if err != nil {
		t.Fatal(err)
	}
	result := b.Exec(context.Background(), "cat /data/message.txt", ExecOptions{})
	if result.ExitCode != 0 || result.Stdout != "from mapfs\n" {
		t.Fatalf("%+v", result)
	}
	result = b.Exec(context.Background(), "echo denied > /new", ExecOptions{})
	if result.ExitCode == 0 || !strings.Contains(result.Stderr, "read-only") {
		t.Fatalf("%+v", result)
	}
}

func TestTimeout(t *testing.T) {
	b, e := New(Options{Limits: Limits{MaxExecutionTime: 10 * time.Millisecond}})
	if e != nil {
		t.Fatal(e)
	}
	r := b.Exec(context.Background(), "sleep 1", ExecOptions{})
	if r.ExitCode != 124 {
		t.Fatalf("%+v", r)
	}
}

func TestSyntaxAndUnknownCommands(t *testing.T) {
	b := newTestBash(t)
	if r := b.Exec(context.Background(), "echo 'oops", ExecOptions{}); r.ExitCode != 2 {
		t.Fatalf("%+v", r)
	}
	if r := b.Exec(context.Background(), "no-such-command", ExecOptions{}); r.ExitCode != 127 {
		t.Fatalf("%+v", r)
	}
}
