package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rumpl/gash/internal/command"
	"github.com/rumpl/gash/internal/commandhelp"
	gfs "github.com/rumpl/gash/pkg/fs"
)

func TestEveryUpstreamHelpDefinitionIsEnforced(t *testing.T) {
	for _, builtin := range Builtins() {
		info, hasHelp := commandhelp.Lookup(builtin.Name)
		if !hasHelp {
			continue
		}
		t.Run(builtin.Name, func(t *testing.T) {
			filesystem := gfs.NewMemory(0)
			if err := filesystem.MkdirAll("work", 0o755); err != nil {
				t.Fatal(err)
			}
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			cwd := "/work"
			ctx := &command.Context{
				FS:     filesystem,
				Cwd:    &cwd,
				Env:    map[string]string{},
				Stdin:  &bytes.Buffer{},
				Stdout: &stdout,
				Stderr: &stderr,
			}
			if exitCode := builtin.Run(context.Background(), []string{"--help"}, ctx); exitCode != 0 {
				t.Fatalf("exit=%d stderr=%q", exitCode, stderr.String())
			}
			if stderr.Len() != 0 {
				t.Fatalf("stderr=%q", stderr.String())
			}
			expectedPrefix := info.Name + " - " + info.Summary + "\n\n"
			if !strings.HasPrefix(stdout.String(), expectedPrefix) {
				t.Fatalf("help output does not start with %q:\n%s", expectedPrefix, stdout.String())
			}
		})
	}
}

func TestFeatureSupportManifestCoversCommandRegistries(t *testing.T) {
	manifest := loadFeatureSupportManifest(t)
	if got, want := manifest.PinnedJustBash.Commit, "2b316eb26b3f3e832e2cf3994d4fef160d5eb8e4"; got != want {
		t.Fatalf("manifest pinned commit = %q, want %q", got, want)
	}

	entries := map[string]featureSupportEntry{}
	for _, entry := range manifest.Commands {
		if entry.Name == "" {
			t.Fatalf("manifest entry with empty name: %#v", entry)
		}
		if _, exists := entries[entry.Name]; exists {
			t.Fatalf("duplicate manifest entry for %q", entry.Name)
		}
		if _, ok := manifest.StatusDefinitions[entry.Status]; !ok {
			t.Fatalf("manifest entry %q uses undefined status %q", entry.Name, entry.Status)
		}
		entries[entry.Name] = entry
	}

	for _, builtin := range Builtins() {
		entry, ok := entries[builtin.Name]
		if !ok {
			t.Fatalf("registered gash built-in %q is missing from docs/status/feature-support.json", builtin.Name)
		}
		if !hasSourcePrefix(entry, "gash") {
			t.Fatalf("manifest entry for registered gash built-in %q does not include a gash source: %#v", builtin.Name, entry.Sources)
		}
	}

	for _, name := range pinnedJustBashRegistryCommands() {
		entry, ok := entries[name]
		if !ok {
			t.Fatalf("pinned just-bash registry command %q is missing from docs/status/feature-support.json", name)
		}
		if !hasSource(entry, "just-bash:pinned") {
			t.Fatalf("manifest entry for pinned just-bash command %q does not include just-bash:pinned source: %#v", name, entry.Sources)
		}
	}

	for _, name := range []string{"fgrep", "sed", "find", "xargs", "diff", "expr", "rg", "awk"} {
		entry, ok := entries[name]
		if !ok {
			t.Fatalf("high-priority search/text command %q is missing from manifest", name)
		}
		if !entry.NearTermPriority || entry.Category != "priority-search-text" {
			t.Fatalf("high-priority search/text command %q is not visibly marked as first-class priority: %#v", name, entry)
		}
	}
}

type featureSupportManifest struct {
	PinnedJustBash    struct{ Commit string } `json:"pinnedJustBash"`
	StatusDefinitions map[string]string       `json:"statusDefinitions"`
	Commands          []featureSupportEntry   `json:"commands"`
}

type featureSupportEntry struct {
	Name             string   `json:"name"`
	Status           string   `json:"status"`
	Category         string   `json:"category"`
	Sources          []string `json:"sources"`
	NearTermPriority bool     `json:"nearTermPriority"`
}

func loadFeatureSupportManifest(t *testing.T) featureSupportManifest {
	t.Helper()
	path := filepath.Join("..", "..", "docs", "status", "feature-support.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read feature support manifest: %v", err)
	}
	var manifest featureSupportManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("parse feature support manifest: %v", err)
	}
	return manifest
}

func hasSource(entry featureSupportEntry, source string) bool {
	for _, candidate := range entry.Sources {
		if candidate == source {
			return true
		}
	}
	return false
}

func hasSourcePrefix(entry featureSupportEntry, prefix string) bool {
	for _, candidate := range entry.Sources {
		if candidate == prefix || strings.HasPrefix(candidate, prefix+":") {
			return true
		}
	}
	return false
}

func pinnedJustBashRegistryCommands() []string {
	return []string{
		"alias", "awk", "base64", "basename", "bash", "cat", "chmod", "clear", "column", "comm",
		"cp", "curl", "cut", "date", "diff", "dirname", "du", "echo", "egrep", "env", "expand",
		"expr", "false", "fgrep", "file", "find", "fold", "grep", "gunzip", "gzip", "head", "help",
		"history", "hostname", "html-to-markdown", "join", "jq", "js-exec", "ln", "ls", "md5sum",
		"mkdir", "mv", "nl", "node", "od", "paste", "printenv", "printf", "pwd", "python", "python3",
		"readlink", "rev", "rg", "rm", "rmdir", "sed", "seq", "sh", "sha1sum", "sha256sum", "sleep",
		"sort", "split", "sqlite3", "stat", "strings", "tac", "tail", "tar", "tee", "time", "timeout",
		"touch", "tr", "tree", "true", "unalias", "unexpand", "uniq", "wc", "which", "whoami", "xan",
		"xargs", "yq", "zcat",
	}
}
