package runtime

import (
	"strings"
	"testing"

	"minimalist/internal/config"
	"minimalist/internal/state"
)

func TestBuildRuntimeConfigFallsBackToDefaultSecret(t *testing.T) {
	paths := Paths{
		ConfigDir:  t.TempDir(),
		DataDir:    t.TempDir(),
		RuntimeDir: t.TempDir(),
	}
	cfg := config.Default()
	cfg.Controller.Secret = ""
	text, err := buildRuntimeConfig(paths, cfg, state.Empty(), nil)
	if err != nil {
		t.Fatalf("build runtime config: %v", err)
	}
	if !strings.Contains(text, `secret: "minimalist-secret"`) {
		t.Fatalf("expected fallback secret in runtime config:\n%s", text)
	}
}

func TestBuildRuntimeConfigIncludesExternalUIAndNameserverPolicy(t *testing.T) {
	paths := Paths{
		ConfigDir:  t.TempDir(),
		DataDir:    t.TempDir(),
		RuntimeDir: t.TempDir(),
	}
	cfg := config.Default()
	text, err := buildRuntimeConfig(paths, cfg, state.Empty(), nil)
	if err != nil {
		t.Fatalf("build runtime config: %v", err)
	}
	for _, needle := range []string{
		"external-ui: " + paths.UIPath(),
		"nameserver-policy:",
		`"geosite:private,cn":`,
		`"+.arpa":`,
	} {
		if !strings.Contains(text, needle) {
			t.Fatalf("missing %q in runtime config:\n%s", needle, text)
		}
	}
}

func TestBuildRuntimeConfigIncludesDNSDefaults(t *testing.T) {
	paths := Paths{
		ConfigDir:  t.TempDir(),
		DataDir:    t.TempDir(),
		RuntimeDir: t.TempDir(),
	}
	cfg := config.Default()
	text, err := buildRuntimeConfig(paths, cfg, state.Empty(), nil)
	if err != nil {
		t.Fatalf("build runtime config: %v", err)
	}
	for _, needle := range []string{
		"default-nameserver:",
		"    - 223.5.5.5",
		"    - 119.29.29.29",
		"direct-nameserver:",
		"    - https://dns.alidns.com/dns-query",
		"    - https://doh.pub/dns-query",
		"  direct-nameserver-follow-policy: true",
		`    - "*.lan"`,
		`    - "connectivitycheck.gstatic.com"`,
	} {
		if !strings.Contains(text, needle) {
			t.Fatalf("missing %q in runtime config:\n%s", needle, text)
		}
	}
}

func TestBuildRuntimeConfigIncludesProfileAndDNSPolicySections(t *testing.T) {
	paths := Paths{
		ConfigDir:  t.TempDir(),
		DataDir:    t.TempDir(),
		RuntimeDir: t.TempDir(),
	}
	cfg := config.Default()
	text, err := buildRuntimeConfig(paths, cfg, state.Empty(), nil)
	if err != nil {
		t.Fatalf("build runtime config: %v", err)
	}
	for _, needle := range []string{
		"profile:\n  store-selected: true\n  store-fake-ip: true\n",
		"  fallback-filter:\n    geoip: false\n",
		"  proxy-server-nameserver:\n    - 223.5.5.5\n    - 119.29.29.29\n",
	} {
		if !strings.Contains(text, needle) {
			t.Fatalf("missing %q in runtime config:\n%s", needle, text)
		}
	}
}

func TestBuildRuntimeConfigIncludesNameserverGeoxAndDNSListen(t *testing.T) {
	paths := Paths{
		ConfigDir:  t.TempDir(),
		DataDir:    t.TempDir(),
		RuntimeDir: t.TempDir(),
	}
	cfg := config.Default()
	text, err := buildRuntimeConfig(paths, cfg, state.Empty(), nil)
	if err != nil {
		t.Fatalf("build runtime config: %v", err)
	}
	for _, needle := range []string{
		"  listen: 0.0.0.0:1053\n",
		"  nameserver:\n    - https://cloudflare-dns.com/dns-query#RULES\n    - https://dns.google/dns-query#RULES\n",
		"geox-url:\n  mmdb: \"https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/country.mmdb\"\n  geoip: \"https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/geoip.dat\"\n  geosite: \"https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/geosite.dat\"\n",
	} {
		if !strings.Contains(text, needle) {
			t.Fatalf("missing %q in runtime config:\n%s", needle, text)
		}
	}
}

func TestBuildRuntimeConfigIncludesBaseNetworkFlags(t *testing.T) {
	paths := Paths{
		ConfigDir:  t.TempDir(),
		DataDir:    t.TempDir(),
		RuntimeDir: t.TempDir(),
	}
	cfg := config.Default()
	text, err := buildRuntimeConfig(paths, cfg, state.Empty(), nil)
	if err != nil {
		t.Fatalf("build runtime config: %v", err)
	}
	for _, needle := range []string{
		"allow-lan: true\n",
		"bind-address: \"*\"\n",
		"log-level: info\n",
		"unified-delay: true\n",
		"tcp-concurrent: true\n",
		"find-process-mode: off\n",
	} {
		if !strings.Contains(text, needle) {
			t.Fatalf("missing %q in runtime config:\n%s", needle, text)
		}
	}
}

func TestBuildRuntimeConfigReflectsIPv6Setting(t *testing.T) {
	paths := Paths{
		ConfigDir:  t.TempDir(),
		DataDir:    t.TempDir(),
		RuntimeDir: t.TempDir(),
	}
	cfg := config.Default()
	cfg.Network.EnableIPv6 = true
	text, err := buildRuntimeConfig(paths, cfg, state.Empty(), nil)
	if err != nil {
		t.Fatalf("build runtime config: %v", err)
	}
	for _, needle := range []string{
		"ipv6: true\n",
		"  ipv6: true\n",
	} {
		if !strings.Contains(text, needle) {
			t.Fatalf("missing %q in runtime config:\n%s", needle, text)
		}
	}
}

func TestBuildRuntimeConfigIncludesGeoFlags(t *testing.T) {
	paths := Paths{
		ConfigDir:  t.TempDir(),
		DataDir:    t.TempDir(),
		RuntimeDir: t.TempDir(),
	}
	cfg := config.Default()
	text, err := buildRuntimeConfig(paths, cfg, state.Empty(), nil)
	if err != nil {
		t.Fatalf("build runtime config: %v", err)
	}
	for _, needle := range []string{
		"geodata-mode: false\n",
		"geo-auto-update: false\n",
		"geo-update-interval: 24\n",
	} {
		if !strings.Contains(text, needle) {
			t.Fatalf("missing %q in runtime config:\n%s", needle, text)
		}
	}
}

func TestBuildRuntimeConfigIncludesDNSBehaviorFlags(t *testing.T) {
	paths := Paths{
		ConfigDir:  t.TempDir(),
		DataDir:    t.TempDir(),
		RuntimeDir: t.TempDir(),
	}
	cfg := config.Default()
	text, err := buildRuntimeConfig(paths, cfg, state.Empty(), nil)
	if err != nil {
		t.Fatalf("build runtime config: %v", err)
	}
	for _, needle := range []string{
		"  use-hosts: true\n",
		"  use-system-hosts: true\n",
		"  cache-algorithm: arc\n",
		"  respect-rules: false\n",
		"  prefer-h3: false\n",
		"  enhanced-mode: fake-ip\n",
		"  fake-ip-range: 198.18.0.1/16\n",
		"  fake-ip-filter-mode: blacklist\n",
	} {
		if !strings.Contains(text, needle) {
			t.Fatalf("missing %q in runtime config:\n%s", needle, text)
		}
	}
}
