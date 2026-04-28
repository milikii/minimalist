package config

import (
	"errors"
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

func TestSaveCreatesMissingParentDirectories(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "etc", "config.yaml")
	cfg := Default()
	cfg.Controller.Secret = "persisted-secret"
	if err := Save(path, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if loaded.Controller.Secret != "persisted-secret" {
		t.Fatalf("expected saved secret, got %q", loaded.Controller.Secret)
	}
}

func TestEnsureCreatesMissingParentDirectories(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "etc", "config.yaml")
	cfg, err := Ensure(path)
	if err != nil {
		t.Fatalf("ensure config: %v", err)
	}
	if cfg.Version != 1 {
		t.Fatalf("expected default config, got %#v", cfg)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected config file to exist: %v", err)
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

func TestEnsureBackfillsExplicitBlankSecret(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	body := "version: 1\nprofile:\n  template: nas-single-lan-v4\n  mode: rule\n  rule_preset: default\ncontroller:\n  bind_address: 127.0.0.1\n  secret: \"\"\n"
	if err := os.WriteFile(path, []byte(body), 0o640); err != nil {
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
	if strings.Contains(string(raw), "secret: \"\"") || !strings.Contains(string(raw), "secret:") {
		t.Fatalf("expected blank secret to be rewritten:\n%s", string(raw))
	}
}

func TestLoadBackfillsMissingSecretInMemoryWithoutPersisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	body := "version: 1\nprofile:\n  template: nas-single-lan-v4\n  mode: rule\n  rule_preset: default\ncontroller:\n  bind_address: 127.0.0.1\n"
	if err := os.WriteFile(path, []byte(body), 0o640); err != nil {
		t.Fatalf("write config: %v", err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config before load: %v", err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Controller.Secret == "" {
		t.Fatalf("expected in-memory secret to be generated")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config after load: %v", err)
	}
	if string(before) != string(after) {
		t.Fatalf("expected load not to rewrite config file")
	}
}

func TestEnsurePreservesExistingSecretWithoutRewriting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	body := "version: 1\nprofile:\n  template: nas-single-lan-v4\n  mode: rule\n  rule_preset: default\ncontroller:\n  bind_address: 127.0.0.1\n  secret: keep-me\n"
	if err := os.WriteFile(path, []byte(body), 0o640); err != nil {
		t.Fatalf("write config: %v", err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat config before ensure: %v", err)
	}
	cfg, err := Ensure(path)
	if err != nil {
		t.Fatalf("ensure config: %v", err)
	}
	if cfg.Controller.Secret != "keep-me" {
		t.Fatalf("expected existing secret, got %q", cfg.Controller.Secret)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat config after ensure: %v", err)
	}
	if !before.ModTime().Equal(after.ModTime()) {
		t.Fatalf("expected ensure not to rewrite config file")
	}
}

func TestEnsureFallsBackToDefaultSecretWhenRandomSourceFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("version: 1\nprofile:\n  template: nas-single-lan-v4\n  mode: rule\n  rule_preset: default\ncontroller:\n  bind_address: 127.0.0.1\n"), 0o640); err != nil {
		t.Fatalf("write config: %v", err)
	}
	oldRandRead := randRead
	randRead = func([]byte) (int, error) {
		return 0, errors.New("rand unavailable")
	}
	defer func() { randRead = oldRandRead }()

	cfg, err := Ensure(path)
	if err != nil {
		t.Fatalf("ensure config: %v", err)
	}
	if cfg.Controller.Secret != "minimalist-secret" {
		t.Fatalf("expected fallback secret, got %q", cfg.Controller.Secret)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !strings.Contains(string(raw), "secret: minimalist-secret") {
		t.Fatalf("expected fallback secret to be persisted:\n%s", string(raw))
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

func TestSaveReturnsErrorWhenTargetPathIsDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir config path: %v", err)
	}
	if err := Save(path, Default()); err == nil || !strings.Contains(err.Error(), "is a directory") {
		t.Fatalf("expected save directory error, got %v", err)
	}
}

func TestPersistedSecretPresentFallsBackToLiteralSecretMarkerOnParseError(t *testing.T) {
	if persistedSecretPresent([]byte("controller: [\nsecret: keep-me\n")) != true {
		t.Fatalf("expected literal secret marker to be treated as present")
	}
	if persistedSecretPresent([]byte("controller: [\nmode: rule\n")) != false {
		t.Fatalf("expected malformed content without secret marker to be treated as missing")
	}
}
