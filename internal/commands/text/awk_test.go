package text

import (
	"bytes"
	"strings"
	"testing"
)

func TestAwkBeginEndFieldsAndPatterns(t *testing.T) {
	program := `BEGIN { print "name", "score" } $2 >= 10 { total += $2; print NR, $1 } END { print "total", total }`
	assertCommand(t, commandAwk, []string{program}, "ada 10\nbob 4\ngrace 12\n", "name score\n1 ada\n3 grace\ntotal 22\n", nil)
}

func TestAwkFieldSeparatorVariablesAndFiles(t *testing.T) {
	program := `{ print prefix ":" $2 }`
	assertCommand(t, commandAwk, []string{"-F,", "-v", "prefix=row", program, "data.csv"}, "", "row:Ada\nrow:Grace\n", map[string]string{"data.csv": "1,Ada\n2,Grace\n"})
}

func TestAwkExpressionsBuiltinsArraysAndFunctions(t *testing.T) {
	program := `function twice(x) { return x * 2 } { split($0, p, ":"); count[p[1]] += twice(p[2]) } END { for (k in count) print k, count[k]; print toupper(substr("héllo", 1, 2)), length("åβ") }`
	assertCommand(t, commandAwk, []string{program}, "a:2\nb:3\na:4\n", "a 12\nb 6\nHÉ 2\n", nil)
}

func TestAwkRangesRegexNextfileAndPrintf(t *testing.T) {
	program := `/start/,/stop/ { if ($1 == "skip") next; printf "%s:%d\n", FILENAME, FNR } FNR == 2 { nextfile }`
	assertCommand(t, commandAwk, []string{program, "a.txt", "b.txt"}, "", "a.txt:1\na.txt:2\nb.txt:1\nb.txt:3\n", map[string]string{
		"a.txt": "start\nstop\nignored\n",
		"b.txt": "start\nskip\nstop\n",
	})
}

func TestAwkUTF8BinaryAndErrors(t *testing.T) {
	assertCommandBytes(t, commandAwk, []string{`{ print length($0), $1 }`}, []byte("åβ \x00x\n"), []byte("5 åβ\n"), nil)

	code, stdout, stderr, _ := runTextCommandBytes(t, commandAwk, []string{`BEGIN { while (1) i++ }`}, nil, nil)
	if code == 0 || len(stdout) != 0 || !strings.Contains(stderr, "loop iteration limit") {
		t.Fatalf("loop limit exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}

	code, stdout, stderr, _ = runTextCommandBytes(t, commandAwk, []string{`BEGIN { print }`, "missing.txt"}, nil, nil)
	if code == 0 || len(stdout) != 0 || !strings.Contains(stderr, "No such file") {
		t.Fatalf("missing file exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	code, stdout, stderr, _ = runTextCommandBytes(t, commandAwk, []string{`BEGIN { system("echo unsafe") }`}, nil, nil)
	if code == 0 || len(stdout) != 0 || !strings.Contains(stderr, "unsupported awk function system") {
		t.Fatalf("unsupported function exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

func TestAwkGetline(t *testing.T) {
	program := `BEGIN { while ((getline line) > 0) print NR, line }`
	assertCommand(t, commandAwk, []string{program}, "x\ny\n", "1 x\n2 y\n", nil)
}

func TestAwkSecurityNoPrototypePollutionNames(t *testing.T) {
	program := `BEGIN { a["__proto__"] = "safe"; a["constructor"] = "still-safe"; for (k in a) print k, a[k] }`
	code, stdout, stderr, _ := runTextCommandBytes(t, commandAwk, []string{program}, nil, nil)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%q", code, stderr)
	}
	if !bytes.Equal(stdout, []byte("__proto__ safe\nconstructor still-safe\n")) {
		t.Fatalf("stdout=%q", stdout)
	}
}
