package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadBackfillsNilSlicesAndVersion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	if err := os.WriteFile(path, []byte("{\"nodes\":null,\"rules\":null,\"acl\":null,\"subscriptions\":null}\n"), 0o640); err != nil {
		t.Fatalf("write state: %v", err)
	}
	st, err := Load(path)
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if st.Version != 1 {
		t.Fatalf("expected version 1, got %d", st.Version)
	}
	if len(st.Nodes) != 0 || len(st.Rules) != 0 || len(st.ACL) != 0 || len(st.Subscriptions) != 0 {
		t.Fatalf("expected empty slices, got %#v", st)
	}
}

func TestEnsureCreatesDefaultState(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	st, err := Ensure(path)
	if err != nil {
		t.Fatalf("ensure state: %v", err)
	}
	if st.Version != 1 || len(st.Nodes) != 0 || len(st.Rules) != 0 || len(st.ACL) != 0 || len(st.Subscriptions) != 0 {
		t.Fatalf("unexpected default state: %#v", st)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	if !strings.Contains(string(raw), "\"version\": 1") {
		t.Fatalf("expected version to be persisted:\n%s", string(raw))
	}
}

func TestSaveWritesTrailingNewline(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	if err := Save(path, Empty()); err != nil {
		t.Fatalf("save state: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	if !strings.HasSuffix(string(raw), "\n") {
		t.Fatalf("expected trailing newline, got %q", string(raw))
	}
}

func TestLoadReportsParseError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	if err := os.WriteFile(path, []byte("{\n"), 0o640); err != nil {
		t.Fatalf("write invalid state: %v", err)
	}
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "parse state") {
		t.Fatalf("expected parse state error, got %v", err)
	}
}

func TestLoadReturnsMissingFileError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "missing.json")
	if _, err := Load(path); err == nil {
		t.Fatalf("expected missing file error")
	}
}

func TestSaveAndEnsureReturnErrorsWhenParentPathIsBlocked(t *testing.T) {
	root := t.TempDir()
	blockedDir := filepath.Join(root, "blocked")
	if err := os.WriteFile(blockedDir, []byte("blocked"), 0o640); err != nil {
		t.Fatalf("write blocking file: %v", err)
	}

	savePath := filepath.Join(blockedDir, "state.json")
	if err := Save(savePath, Empty()); err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("expected save error, got %v", err)
	}

	ensurePath := filepath.Join(root, "blocked", "nested", "state.json")
	if _, err := Ensure(ensurePath); err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("expected ensure error, got %v", err)
	}
}

func TestNowISOReturnsRFC3339(t *testing.T) {
	ts := NowISO()
	if _, err := time.Parse(time.RFC3339, ts); err != nil {
		t.Fatalf("expected RFC3339 timestamp, got %q: %v", ts, err)
	}
}
