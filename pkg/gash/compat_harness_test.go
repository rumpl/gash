package gash_test

import (
	"strings"
	"testing"
	"time"

	"github.com/rumpl/gash/internal/gashtest"
	"github.com/rumpl/gash/pkg/gash"
)

// These representative compatibility smoke tests are Go-owned adaptations of
// public just-bash Bash execution scenarios pinned at commit
// 2b316eb26b3f3e832e2cf3994d4fef160d5eb8e4:
//   - Bash.general.test.ts for variables, functions, loops, command
//     substitution, stdin, env, cwd, and filesystem behavior.
//   - Bash.commands.test.ts for pipelines, redirections, shell commands, and
//     failures.
//   - Bash.exec-options.test.ts and Bash.exec-args-forwarding.test.ts for
//     execution options such as args, env, cwd, stdin, and limits.
//
// The goal is a compact harness target for future upstream-derived cases, not a
// full differential runner against just-bash.
func TestJustBashStyleCompatibilityHarness(t *testing.T) {
	gashtest.RunAll(t, []gashtest.Case{
		{
			Name:       "pipeline filters stdin and environment",
			Script:     `cat | grep "$NEEDLE" | sort`,
			Env:        map[string]string{"NEEDLE": "apple"},
			Stdin:      "pear\napple\nbanana\napple pie\n",
			WantStdout: "apple\napple pie\n",
		},
		{
			Name: "redirections append and read from cwd",
			Files: map[string]string{
				"work/input.txt": "first\n",
			},
			Cwd:        "/work",
			Script:     `cat input.txt > output.txt; echo second >> output.txt; wc -l < output.txt`,
			WantStdout: "2\n",
			WantFiles:  map[string]string{"/work/output.txt": "first\nsecond\n"},
		},
		{
			Name: "variables loops functions and command substitution",
			Script: `
name=world
greet() { printf 'hello %s\n' "$1"; }
total=0
for n in 1 2 3; do total=$((total + n)); done
if [[ $total -eq 6 ]]; then greet "$(printf '%s' "$name")"; fi
`,
			WantStdout: "hello world\n",
		},
		{
			Name:       "command failure reports status stderr and recovery branch",
			Script:     `false || echo recovered; no-such-command`,
			WantStdout: "recovered\n",
			WantStderr: "bash: no-such-command: command not found\n",
			WantStatus: 127,
		},
		{
			Name:       "nested shell executes generated script",
			Script:     `echo 'echo child-nested' > child.sh; bash child.sh`,
			WantStdout: "child-nested\n",
			WantFiles:  map[string]string{"/home/user/child.sh": "echo child-nested\n"},
		},
		{
			Name:       "top-level args are available to script",
			Script:     `printf '%s/%s/%s\n' "$0" "$1" "$2"`,
			Args:       []string{"left", "right"},
			WantStdout: "gosh/left/right\n",
		},
		{
			Name: "filesystem mutation creates moves and removes files",
			Script: `
mkdir -p out/sub
printf data > out/sub/a
cp out/sub/a out/sub/b
rm out/sub/a
mv out/sub/b out/final
`,
			WantFiles:   map[string]string{"/home/user/out/final": "data"},
			WantMissing: []string{"/home/user/out/sub/a", "/home/user/out/sub/b"},
			Check: func(t testing.TB, shell *gash.Bash, _ gash.Result) {
				t.Helper()
				tree, err := gashtest.Tree(shell, "/home/user/out")
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(tree, "/home/user/out/final") || !strings.Contains(tree, "/home/user/out/sub") {
					t.Fatalf("unexpected tree:\n%s", tree)
				}
			},
		},
		{
			Name:       "execution timeout limit",
			Script:     `sleep 1`,
			Limits:     gash.Limits{MaxExecutionTime: 10 * time.Millisecond},
			WantStderr: "bash: execution timed out\n",
			WantStatus: 124,
		},
	})
}
