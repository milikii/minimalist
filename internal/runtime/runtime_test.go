package runtime

import (
	"os"
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

func TestBuildRuntimeConfigUsesConfiguredSecret(t *testing.T) {
	paths := Paths{
		ConfigDir:  t.TempDir(),
		DataDir:    t.TempDir(),
		RuntimeDir: t.TempDir(),
	}
	cfg := config.Default()
	cfg.Controller.Secret = "custom-secret"
	text, err := buildRuntimeConfig(paths, cfg, state.Empty(), nil)
	if err != nil {
		t.Fatalf("build runtime config: %v", err)
	}
	if !strings.Contains(text, `secret: "custom-secret"`) {
		t.Fatalf("expected configured secret in runtime config:\n%s", text)
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

func TestBuildRuntimeConfigIncludesSubscriptionProvidersWhenEnabled(t *testing.T) {
	paths := Paths{
		ConfigDir:  t.TempDir(),
		DataDir:    t.TempDir(),
		RuntimeDir: t.TempDir(),
	}
	cfg := config.Default()
	st := state.Empty()
	st.Subscriptions = []state.Subscription{{
		ID:        "sub-1",
		Name:      "sub-1",
		URL:       "https://subscription.example.com/sub.txt",
		Enabled:   true,
		CreatedAt: state.NowISO(),
	}}
	st.Nodes = []state.Node{{
		ID:         "1",
		Name:       "sub-node",
		Enabled:    true,
		URI:        "trojan://password@example.org:443?security=tls#sub-node",
		ImportedAt: state.NowISO(),
		Source:     state.Source{Kind: "subscription", ID: "sub-1"},
	}}
	if err := os.MkdirAll(paths.SubscriptionDir(), 0o755); err != nil {
		t.Fatalf("mkdir subscription dir: %v", err)
	}
	if err := os.WriteFile(paths.SubscriptionFile("sub-1"), []byte("trojan://password@example.org:443?security=tls#sub-node\n"), 0o640); err != nil {
		t.Fatalf("write subscription cache: %v", err)
	}
	text, err := buildRuntimeConfig(paths, cfg, st, nil)
	if err != nil {
		t.Fatalf("build runtime config: %v", err)
	}
	for _, needle := range []string{
		"proxy-providers:",
		"  subscription-sub:\n    type: file\n    path: ./proxy_providers/subscriptions/sub-1.txt\n",
		"    - subscription-sub\n",
	} {
		if !strings.Contains(text, needle) {
			t.Fatalf("missing %q in runtime config:\n%s", needle, text)
		}
	}
}

func TestBuildRuntimeConfigIncludesPortAndModeHeaders(t *testing.T) {
	paths := Paths{
		ConfigDir:  t.TempDir(),
		DataDir:    t.TempDir(),
		RuntimeDir: t.TempDir(),
	}
	cfg := config.Default()
	cfg.Profile.Mode = "global"
	text, err := buildRuntimeConfig(paths, cfg, state.Empty(), nil)
	if err != nil {
		t.Fatalf("build runtime config: %v", err)
	}
	for _, needle := range []string{
		"mixed-port: 7890\n",
		"tproxy-port: 7893\n",
		"mode: global\n",
		"external-controller: 127.0.0.1:19090\n",
	} {
		if !strings.Contains(text, needle) {
			t.Fatalf("missing %q in runtime config:\n%s", needle, text)
		}
	}
}

func TestBuildRuntimeConfigUsesConfiguredExternalController(t *testing.T) {
	paths := Paths{
		ConfigDir:  t.TempDir(),
		DataDir:    t.TempDir(),
		RuntimeDir: t.TempDir(),
	}
	cfg := config.Default()
	cfg.Controller.BindAddress = "0.0.0.0"
	cfg.Ports.Controller = 29090
	text, err := buildRuntimeConfig(paths, cfg, state.Empty(), nil)
	if err != nil {
		t.Fatalf("build runtime config: %v", err)
	}
	if !strings.Contains(text, "external-controller: 0.0.0.0:29090\n") {
		t.Fatalf("expected configured external controller in runtime config:\n%s", text)
	}
}

func TestBuildRuntimeConfigIncludesLANAllowedIPs(t *testing.T) {
	paths := Paths{
		ConfigDir:  t.TempDir(),
		DataDir:    t.TempDir(),
		RuntimeDir: t.TempDir(),
	}
	cfg := config.Default()
	cfg.Network.LANCIDRs = []string{"10.0.0.0/24", "10.0.1.0/24"}
	text, err := buildRuntimeConfig(paths, cfg, state.Empty(), nil)
	if err != nil {
		t.Fatalf("build runtime config: %v", err)
	}
	for _, needle := range []string{
		"lan-allowed-ips:",
		"  - 10.0.0.0/24\n",
		"  - 10.0.1.0/24\n",
		"  - 127.0.0.0/8\n",
	} {
		if !strings.Contains(text, needle) {
			t.Fatalf("missing %q in runtime config:\n%s", needle, text)
		}
	}
}

func TestBuildRuntimeConfigIncludesLANDisallowedIPsWhenConfigured(t *testing.T) {
	paths := Paths{
		ConfigDir:  t.TempDir(),
		DataDir:    t.TempDir(),
		RuntimeDir: t.TempDir(),
	}
	cfg := config.Default()
	cfg.Access.LANDisallowedCIDRs = []string{"10.10.10.0/24", "172.16.0.0/16"}
	text, err := buildRuntimeConfig(paths, cfg, state.Empty(), nil)
	if err != nil {
		t.Fatalf("build runtime config: %v", err)
	}
	for _, needle := range []string{
		"lan-disallowed-ips:",
		"  - 10.10.10.0/24\n",
		"  - 172.16.0.0/16\n",
	} {
		if !strings.Contains(text, needle) {
			t.Fatalf("missing %q in runtime config:\n%s", needle, text)
		}
	}
}

func TestBuildRuntimeConfigIncludesControllerCorsWhenEnabled(t *testing.T) {
	paths := Paths{
		ConfigDir:  t.TempDir(),
		DataDir:    t.TempDir(),
		RuntimeDir: t.TempDir(),
	}
	cfg := config.Default()
	cfg.Controller.CORSAllowOrigins = []string{"https://panel.example"}
	cfg.Controller.CORSAllowPrivateNetwork = true
	text, err := buildRuntimeConfig(paths, cfg, state.Empty(), nil)
	if err != nil {
		t.Fatalf("build runtime config: %v", err)
	}
	for _, needle := range []string{
		"external-controller-cors:",
		"  allow-origins:\n    - \"https://panel.example\"\n",
		"  allow-private-network: true\n",
	} {
		if !strings.Contains(text, needle) {
			t.Fatalf("missing %q in runtime config:\n%s", needle, text)
		}
	}
}

func TestBuildRuntimeConfigIncludesAuthSectionsWhenEnabled(t *testing.T) {
	paths := Paths{
		ConfigDir:  t.TempDir(),
		DataDir:    t.TempDir(),
		RuntimeDir: t.TempDir(),
	}
	cfg := config.Default()
	cfg.Access.Authentication = []string{"user:pass"}
	cfg.Access.SkipAuthPrefixes = []string{"192.168.2."}
	text, err := buildRuntimeConfig(paths, cfg, state.Empty(), nil)
	if err != nil {
		t.Fatalf("build runtime config: %v", err)
	}
	for _, needle := range []string{
		"authentication:",
		"  - \"user:pass\"\n",
		"skip-auth-prefixes:",
		"  - 192.168.2.\n",
	} {
		if !strings.Contains(text, needle) {
			t.Fatalf("missing %q in runtime config:\n%s", needle, text)
		}
	}
}

func TestBuildRuntimeConfigIncludesManualProviderWhenNodesEnabled(t *testing.T) {
	paths := Paths{
		ConfigDir:  t.TempDir(),
		DataDir:    t.TempDir(),
		RuntimeDir: t.TempDir(),
	}
	cfg := config.Default()
	st := state.Empty()
	st.Nodes = []state.Node{{
		ID:         "1",
		Name:       "manual-1",
		Enabled:    true,
		URI:        "trojan://password@example.org:443?security=tls#manual-1",
		ImportedAt: state.NowISO(),
		Source:     state.Source{Kind: "manual"},
	}}
	text, err := buildRuntimeConfig(paths, cfg, st, nil)
	if err != nil {
		t.Fatalf("build runtime config: %v", err)
	}
	for _, needle := range []string{
		"proxy-providers:",
		"  manual:\n    type: file\n    path: ./proxy_providers/manual.txt\n",
		"proxy-groups:\n  - name: \"PROXY\"\n    type: select\n    proxies:\n      - DIRECT\n      - AUTO\n    use:\n      - manual\n",
	} {
		if !strings.Contains(text, needle) {
			t.Fatalf("missing %q in runtime config:\n%s", needle, text)
		}
	}
}
