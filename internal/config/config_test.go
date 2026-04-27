package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureAndLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	cfg, err := Ensure(path)
	if err != nil {
		t.Fatalf("ensure config: %v", err)
	}
	if cfg.Profile.Template != "nas-single-lan-v4" {
		t.Fatalf("unexpected template: %s", cfg.Profile.Template)
	}
	cfg.Network.LANInterfaces = []string{"br0", "br1"}
	if err := Save(path, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if len(loaded.Network.LANInterfaces) != 2 || loaded.Network.LANInterfaces[0] != "br0" || loaded.Network.LANInterfaces[1] != "br1" {
		t.Fatalf("round trip interfaces mismatch: %#v", loaded.Network.LANInterfaces)
	}
	if loaded.Controller.Secret == "" {
		t.Fatalf("secret should not be empty")
	}
}

func TestEnsureBackfillsMissingSecret(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("version: 1\nprofile:\n  template: nas-single-lan-v4\n  mode: rule\n  rule_preset: default\ncontroller:\n  bind_address: 127.0.0.1\n"), 0o640); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, err := Ensure(path)
	if err != nil {
		t.Fatalf("ensure config: %v", err)
	}
	if cfg.Controller.Secret == "" {
		t.Fatalf("expected generated secret")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !strings.Contains(string(raw), "secret:") {
		t.Fatalf("expected secret to be persisted:\n%s", string(raw))
	}
}

func TestLoadReportsParseError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("version: [\n"), 0o640); err != nil {
		t.Fatalf("write invalid config: %v", err)
	}
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "parse config") {
		t.Fatalf("expected parse config error, got %v", err)
	}
}

func TestLoadReturnsMissingFileError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "missing.yaml")
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

	savePath := filepath.Join(blockedDir, "config.yaml")
	if err := Save(savePath, Default()); err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("expected save error, got %v", err)
	}

	ensurePath := filepath.Join(root, "blocked", "nested", "config.yaml")
	if _, err := Ensure(ensurePath); err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("expected ensure error, got %v", err)
	}
}
