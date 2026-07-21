package data

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/rumpl/gash/internal/command"
	gfs "github.com/rumpl/gash/pkg/fs"
)

func runDataCommand(t *testing.T, run command.Func, args []string, stdin string, files map[string]string) (int, string, string) {
	t.Helper()
	fsys := gfs.NewMemory(0)
	if err := fsys.MkdirAll("work", 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range files {
		if err := fsys.WriteFile("work/"+name, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var out, stderr bytes.Buffer
	cwd := "/work"
	ctx := &command.Context{FS: fsys, Cwd: &cwd, Env: map[string]string{"NAME": "gash"}, Stdin: strings.NewReader(stdin), Stdout: &out, Stderr: &stderr}
	code := run(context.Background(), args, ctx)
	return code, out.String(), stderr.String()
}

func TestJQBasicFiltersRawInputArgsAndUTF8(t *testing.T) {
	code, out, err := runDataCommand(t, commandJQ, []string{".items[] | .name"}, `{"items":[{"name":"éclair"},{"name":"茶"}]}`, nil)
	if code != 0 || err != "" || !strings.Contains(out, "éclair") || !strings.Contains(out, "茶") {
		t.Fatalf("jq basic code=%d out=%q err=%q", code, out, err)
	}
	code, out, err = runDataCommand(t, commandJQ, []string{"-R", "-s", ". | split(\"\\n\") | .[0]"}, "alpha\nbeta\n", nil)
	if code != 0 || out != "\"alpha\"\n" || err != "" {
		t.Fatalf("jq raw/slurp code=%d out=%q err=%q", code, out, err)
	}
	code, out, err = runDataCommand(t, commandJQ, []string{"-n", "-r", "--arg", "who", "world", "$who"}, "", nil)
	if code != 0 || out != "world\n" || err != "" {
		t.Fatalf("jq arg code=%d out=%q err=%q", code, out, err)
	}
}

func TestJQRejectsInvalidJSONAndHostModuleLoading(t *testing.T) {
	code, _, err := runDataCommand(t, commandJQ, []string{"."}, `{bad`, nil)
	if code == 0 || !strings.Contains(err, "invalid") {
		t.Fatalf("jq invalid code=%d err=%q", code, err)
	}
	code, _, err = runDataCommand(t, commandJQ, []string{`include "secret"; .`}, `{}`, map[string]string{"secret.jq": "."})
	if code == 0 || !strings.Contains(err, "module") {
		t.Fatalf("jq module loading code=%d err=%q", code, err)
	}
}

func TestYQNavigationEnvMultiDocAndUTF8(t *testing.T) {
	input := "name: gash\nitems:\n  - é\n---\nname: other\nitems: [茶]\n"
	code, out, err := runDataCommand(t, commandYQ, []string{"-r", ".items[0]"}, input, nil)
	if code != 0 || out != "é\n茶\n" || err != "" {
		t.Fatalf("yq nav code=%d out=%q err=%q", code, out, err)
	}
	code, out, err = runDataCommand(t, commandYQ, []string{"-o=json", `env.NAME`}, "{}", nil)
	if code != 0 || out != "\"gash\"\n" || err != "" {
		t.Fatalf("yq env code=%d out=%q err=%q", code, out, err)
	}
}

func TestYQRejectsUnsafeOrMalformedYaml(t *testing.T) {
	code, _, err := runDataCommand(t, commandYQ, []string{"."}, "a: [unterminated\n", nil)
	if code == 0 || err == "" {
		t.Fatalf("yq malformed code=%d err=%q", code, err)
	}
	code, out, err := runDataCommand(t, commandYQ, []string{"."}, "__proto__:\n  polluted: true\n", nil)
	if code != 0 || err != "" || !strings.Contains(out, "__proto__") {
		t.Fatalf("yq proto data code=%d out=%q err=%q", code, out, err)
	}
}

func TestXanCSVSelectFilterSortMapAggGroupViewUTF8(t *testing.T) {
	csv := "name,city,score\nAda,Zürich,2\nBob,東京,10\n"
	code, out, err := runDataCommand(t, commandXan, []string{"headers"}, csv, nil)
	if code != 0 || out != "name\ncity\nscore\n" || err != "" {
		t.Fatalf("headers %d %q %q", code, out, err)
	}
	code, out, _ = runDataCommand(t, commandXan, []string{"select", "--select", "city,name"}, csv, nil)
	if code != 0 || !strings.Contains(out, "Zürich,Ada") {
		t.Fatalf("select %d %q", code, out)
	}
	code, out, _ = runDataCommand(t, commandXan, []string{"filter", "--filter", "score>2"}, csv, nil)
	if code != 0 || strings.Contains(out, "Ada") || !strings.Contains(out, "東京") {
		t.Fatalf("filter %d %q", code, out)
	}
	code, out, _ = runDataCommand(t, commandXan, []string{"sort", "--select", "score", "--numeric", "--reverse"}, csv, nil)
	if code != 0 || !strings.Contains(strings.Split(out, "\n")[1], "Bob") {
		t.Fatalf("sort %d %q", code, out)
	}
	code, out, _ = runDataCommand(t, commandXan, []string{"map", "--map", "label=name+city"}, csv, nil)
	if code != 0 || !strings.Contains(out, "AdaZürich") {
		t.Fatalf("map %d %q", code, out)
	}
	code, out, _ = runDataCommand(t, commandXan, []string{"agg", "--agg", "sum(score)"}, csv, nil)
	if code != 0 || !strings.Contains(out, "12") {
		t.Fatalf("agg %d %q", code, out)
	}
	code, out, _ = runDataCommand(t, commandXan, []string{"groupby", "--groupby", "city"}, csv, nil)
	if code != 0 || !strings.Contains(out, "Zürich,1") {
		t.Fatalf("group %d %q", code, out)
	}
	code, out, _ = runDataCommand(t, commandXan, []string{"view"}, csv, nil)
	if code != 0 || !strings.Contains(out, "score") || !strings.Contains(out, "東京") {
		t.Fatalf("view %d %q", code, out)
	}
}

func TestXanMalformedAndUnsupportedAreBoundedErrors(t *testing.T) {
	code, _, err := runDataCommand(t, commandXan, []string{"select", "--select", "a"}, "a\n\"unterminated", nil)
	if code == 0 || err == "" {
		t.Fatalf("xan malformed code=%d err=%q", code, err)
	}
	code, _, err = runDataCommand(t, commandXan, []string{"explode"}, "a\n1\n", nil)
	if code == 0 || !strings.Contains(err, "unsupported") {
		t.Fatalf("xan unsupported code=%d err=%q", code, err)
	}
}

func TestHTMLToMarkdownBasicAndUnsafeLinks(t *testing.T) {
	html := `<h1>Title</h1><p>Hello <strong>gash</strong> <a href="/ok">link</a><a href="javascript:alert(1)">bad</a></p><ul><li>one</li><li>茶</li></ul><script>alert(1)</script>`
	code, out, err := runDataCommand(t, commandHTMLToMarkdown, nil, html, nil)
	if code != 0 || err != "" {
		t.Fatalf("html code=%d err=%q", code, err)
	}
	for _, want := range []string{"# Title", "**gash**", "[link](/ok)", "- 茶"} {
		if !strings.Contains(out, want) {
			t.Fatalf("markdown missing %q in %q", want, out)
		}
	}
	if strings.Contains(out, "javascript:") || strings.Contains(out, "alert") {
		t.Fatalf("unsafe content in %q", out)
	}
}
