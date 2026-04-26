package config

import (
	"path/filepath"
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
