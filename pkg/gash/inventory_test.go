package gash

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/rumpl/gash/internal/commands"
	"github.com/rumpl/gash/pkg/network"
)

// TestCommandInventoryConsistency keeps the implementation registries and the
// user-facing status manifest in sync. Shell-native commands are intentionally
// included here because they do not appear in internal/commands.Builtins.
func TestCommandInventoryConsistency(t *testing.T) {
	manifest := loadCommandInventoryManifest(t)
	entries := make(map[string]commandInventoryEntry, len(manifest.Commands))
	for _, entry := range manifest.Commands {
		if entry.Name == "" {
			t.Fatal("docs/status/feature-support.json contains a command with an empty name")
		}
		if previous, exists := entries[entry.Name]; exists {
			t.Fatalf("docs/status/feature-support.json lists command %q twice (statuses %q and %q)", entry.Name, previous.Status, entry.Status)
		}
		if _, exists := manifest.StatusDefinitions[entry.Status]; !exists {
			t.Fatalf("documented command %q has undefined status %q", entry.Name, entry.Status)
		}
		entries[entry.Name] = entry
	}

	ordinary := commandNameSet(commands.Builtins())
	networkEnabled := commandNameSet(commands.BuiltinsWithNetwork(&network.Policy{}))

	for name := range ordinary {
		entry, documented := entries[name]
		if !documented {
			t.Errorf("registered command %q is missing from docs/status/feature-support.json; add a manifest entry", name)
			continue
		}
		if !implementedStatus(entry.Status) {
			t.Errorf("registered command %q has status %q; use core, useful, or partial", name, entry.Status)
		}
	}

	// These names are handled by the interpreter/discovery layer rather than the
	// ordinary command registry. Keep this derived from discovery.go so additions
	// there automatically participate in the reverse manifest check.
	shellNative := make(map[string]bool, len(discoverableShellBuiltins)+3)
	for name := range discoverableShellBuiltins {
		shellNative[name] = true
	}
	for _, name := range []string{"bash", "sh", "kill"} {
		shellNative[name] = true
	}

	for name, entry := range entries {
		if implementedStatus(entry.Status) && !ordinary[name] && !shellNative[name] {
			t.Errorf("documented command %q is marked %q but is not registered or shell-native; register it or update its status", name, entry.Status)
		}
	}

	optionalRuntimes := map[string]bool{"python": true, "python3": true, "node": true, "js-exec": true}
	for name, entry := range entries {
		if entry.Status != "optional" {
			continue
		}
		if ordinary[name] {
			t.Errorf("optional command %q is registered by default; keep it capability-scoped and update the manifest if it becomes ordinary", name)
		}
		if name == "curl" {
			if !networkEnabled[name] {
				t.Error("optional command \"curl\" is not registered by BuiltinsWithNetwork(policy)")
			}
			continue
		}
		if !optionalRuntimes[name] {
			t.Errorf("optional command %q is not an acknowledged runtime boundary; document how it is capability-scoped in this guard", name)
		}
		if networkEnabled[name] {
			t.Errorf("optional runtime command %q is unexpectedly registered with network policy", name)
		}
	}

	for name := range networkEnabled {
		if ordinary[name] {
			continue
		}
		entry, documented := entries[name]
		if !documented {
			t.Errorf("network-enabled command %q is missing from docs/status/feature-support.json", name)
		} else if name != "curl" || entry.Status != "optional" {
			t.Errorf("network-only command %q must be explicitly accounted for (currently only optional curl is expected); manifest status is %q", name, entry.Status)
		}
	}
}

func TestDocumentedCommandListsMatchManifest(t *testing.T) {
	manifest := loadCommandInventoryManifest(t)
	byStatus := map[string]map[string]bool{
		"core": {}, "useful": {}, "partial": {},
	}
	readmeCommands := map[string]bool{}
	statusCounts := map[string]int{}
	for _, entry := range manifest.Commands {
		statusCounts[entry.Status]++
		if names, ok := byStatus[entry.Status]; ok {
			names[entry.Name] = true
			readmeCommands[entry.Name] = true
		} else if entry.Status == "optional" && entry.Name == "curl" {
			readmeCommands[entry.Name] = true
		}
	}

	root := filepath.Join("..", "..")
	readme := readDocumentationFile(t, filepath.Join(root, "README.md"))
	assertDocumentedNames(t, "README current registered built-ins", markdownCommandParagraph(t, readme, "Current registered built-ins include:"), readmeCommands)

	porting := readDocumentationFile(t, filepath.Join(root, "PORTING.md"))
	for _, status := range []string{"core", "useful", "partial"} {
		heading := "### " + strings.ToUpper(status[:1]) + status[1:]
		assertDocumentedNames(t, "PORTING "+status, markdownCommandParagraph(t, porting, heading), byStatus[status])
	}
	countSummary := fmt.Sprintf("**%d commands**: **%d available by\ndefault** (**%d core**, **%d useful**, and **%d partial**) plus **%d optional**", len(manifest.Commands), statusCounts["core"]+statusCounts["useful"]+statusCounts["partial"], statusCounts["core"], statusCounts["useful"], statusCounts["partial"], statusCounts["optional"])
	if !strings.Contains(porting, countSummary) {
		t.Fatalf("PORTING command count summary is stale; want %q", countSummary)
	}
}

var markdownCommandName = regexp.MustCompile("`([a-z0-9][a-z0-9-]*)`")

func readDocumentationFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func markdownCommandParagraph(t *testing.T, document, marker string) map[string]bool {
	t.Helper()
	start := strings.Index(document, marker)
	if start < 0 {
		t.Fatalf("documentation marker %q not found", marker)
	}
	remainder := document[start+len(marker):]
	paragraphs := strings.Split(remainder, "\n\n")
	for _, paragraph := range paragraphs {
		if !strings.HasPrefix(strings.TrimSpace(paragraph), "`") && !strings.HasPrefix(strings.TrimSpace(paragraph), "- `") {
			continue
		}
		names := map[string]bool{}
		for _, match := range markdownCommandName.FindAllStringSubmatch(paragraph, -1) {
			if names[match[1]] {
				t.Fatalf("%s contains duplicate command %q", marker, match[1])
			}
			names[match[1]] = true
		}
		return names
	}
	t.Fatalf("no command paragraph follows documentation marker %q", marker)
	return nil
}

func assertDocumentedNames(t *testing.T, label string, got, want map[string]bool) {
	t.Helper()
	for name := range want {
		if !got[name] {
			t.Errorf("%s is missing %q", label, name)
		}
	}
	for name := range got {
		if !want[name] {
			t.Errorf("%s contains stale or misplaced command %q", label, name)
		}
	}
}

func commandNameSet(registry []commands.Command) map[string]bool {
	names := make(map[string]bool, len(registry))
	for _, command := range registry {
		names[command.Name] = true
	}
	return names
}

func implementedStatus(status string) bool {
	return status == "core" || status == "useful" || status == "partial"
}

type commandInventoryManifest struct {
	StatusDefinitions map[string]string       `json:"statusDefinitions"`
	Commands          []commandInventoryEntry `json:"commands"`
}

type commandInventoryEntry struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

func loadCommandInventoryManifest(t *testing.T) commandInventoryManifest {
	t.Helper()
	path := filepath.Join("..", "..", "docs", "status", "feature-support.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var manifest commandInventoryManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return manifest
}
