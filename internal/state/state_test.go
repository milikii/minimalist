package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
