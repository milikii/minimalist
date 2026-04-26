package rulesrepo

import (
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
