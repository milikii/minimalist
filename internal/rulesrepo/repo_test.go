package rulesrepo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitAndRenderDefaultRepo(t *testing.T) {
	root := filepath.Join(t.TempDir(), "rules-repo", "default")
	if err := InitDefaultRepo(root); err != nil {
		t.Fatalf("init repo: %v", err)
	}
	manifest := filepath.Join(root, "manifest.yaml")
	lines, err := Render(manifest)
	if err != nil {
		t.Fatalf("render repo: %v", err)
	}
	if len(lines) == 0 {
		t.Fatalf("expected rendered rules")
	}
	if !strings.Contains(strings.Join(lines, "\n"), "DOMAIN-SUFFIX,smzdm.com,DIRECT") {
		t.Fatalf("missing expected direct rule")
	}
	search, err := Search(manifest, "google")
	if err != nil {
		t.Fatalf("search repo: %v", err)
	}
	if len(search) < 2 {
		t.Fatalf("expected search results, got %#v", search)
	}
}

func TestLoadManifestAndReadEntriesValidateInputs(t *testing.T) {
	dir := t.TempDir()
	manifest := filepath.Join(dir, "manifest.yaml")
	if err := os.WriteFile(manifest, []byte("rulesets: []\n"), 0o640); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if _, err := LoadManifest(manifest); err == nil || !strings.Contains(err.Error(), "empty manifest") {
		t.Fatalf("expected empty manifest error, got %v", err)
	}
	entriesPath := filepath.Join(dir, "entries.txt")
	if err := os.WriteFile(entriesPath, []byte("# comment\nexample.com\n\nexample.com\n"), 0o640); err != nil {
		t.Fatalf("write entries: %v", err)
	}
	if _, err := ReadEntries(entriesPath); err == nil || !strings.Contains(err.Error(), "duplicate rule entry") {
		t.Fatalf("expected duplicate entry error, got %v", err)
	}
	if err := ValidateEntry("domain", "bad value", entriesPath); err == nil || !strings.Contains(err.Error(), "invalid domain entry") {
		t.Fatalf("expected invalid domain error, got %v", err)
	}
	if err := ValidateEntry("unsupported", "example.com", entriesPath); err == nil || !strings.Contains(err.Error(), "unsupported rule type") {
		t.Fatalf("expected unsupported rule type error, got %v", err)
	}
}

func TestFindRulesetAndDescribeRulesetReportMissingPaths(t *testing.T) {
	dir := t.TempDir()
	manifest := filepath.Join(dir, "manifest.yaml")
	if err := os.WriteFile(manifest, []byte("rulesets:\n  - name: test\n    category: demo\n    type: domain\n    source: missing.txt\n    target: direct\n"), 0o640); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if _, _, err := FindRuleset(manifest, "unknown"); err == nil || !strings.Contains(err.Error(), "unknown ruleset: unknown") {
		t.Fatalf("expected unknown ruleset error, got %v", err)
	}
	if _, _, err := FindRuleset(manifest, "test"); err == nil || !strings.Contains(err.Error(), "missing source") {
		t.Fatalf("expected missing source error, got %v", err)
	}
	if _, err := DescribeRuleset(manifest, "test"); err == nil || !strings.Contains(err.Error(), "missing source") {
		t.Fatalf("expected describe missing source error, got %v", err)
	}
}

func TestAppendAndRemoveEntryIndexDeduplicateAndRewrite(t *testing.T) {
	dir := t.TempDir()
	manifest := filepath.Join(dir, "manifest.yaml")
	source := filepath.Join(dir, "entries.txt")
	if err := os.WriteFile(manifest, []byte("rulesets:\n  - name: test\n    category: demo\n    type: domain\n    source: entries.txt\n    target: direct\n"), 0o640); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if err := os.WriteFile(source, nil, 0o640); err != nil {
		t.Fatalf("write empty source: %v", err)
	}
	if err := AppendEntry(manifest, "test", "example.com"); err != nil {
		t.Fatalf("append entry: %v", err)
	}
	if err := AppendEntry(manifest, "test", "example.com"); err != nil {
		t.Fatalf("append duplicate entry: %v", err)
	}
	lines, err := ReadEntries(source)
	if err != nil {
		t.Fatalf("read entries: %v", err)
	}
	if len(lines) != 1 || lines[0] != "example.com" {
		t.Fatalf("unexpected entries after append: %#v", lines)
	}
	if err := RemoveEntryIndex(manifest, "test", 1); err != nil {
		t.Fatalf("remove entry index: %v", err)
	}
	lines, err = ReadEntries(source)
	if err != nil {
		t.Fatalf("read entries after remove: %v", err)
	}
	if len(lines) != 0 {
		t.Fatalf("expected empty entries after remove, got %#v", lines)
	}
}
