package runtime

import (
	"os"
	"path/filepath"
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

func TestBuildRuntimeConfigOmitsControllerCORSWhenDisabled(t *testing.T) {
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
	if strings.Contains(text, "external-controller-cors:") {
		t.Fatalf("did not expect controller cors section by default:\n%s", text)
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

func TestBuildRuntimeConfigIncludesDNSEnableAndFallback(t *testing.T) {
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
		"  enable: true\n",
		"  fallback: []\n",
		"  direct-nameserver-follow-policy: true\n",
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

func TestBuildRuntimeConfigOmitsAuthenticationWhenDisabled(t *testing.T) {
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
	if strings.Contains(text, "authentication:") || strings.Contains(text, "skip-auth-prefixes:") {
		t.Fatalf("did not expect auth sections by default:\n%s", text)
	}
}

func TestBuildRuntimeConfigOmitsSkipAuthPrefixesWithoutAuthentication(t *testing.T) {
	paths := Paths{
		ConfigDir:  t.TempDir(),
		DataDir:    t.TempDir(),
		RuntimeDir: t.TempDir(),
	}
	cfg := config.Default()
	cfg.Access.SkipAuthPrefixes = []string{"192.168.2."}
	text, err := buildRuntimeConfig(paths, cfg, state.Empty(), nil)
	if err != nil {
		t.Fatalf("build runtime config: %v", err)
	}
	if strings.Contains(text, "skip-auth-prefixes:") {
		t.Fatalf("did not expect skip-auth-prefixes without authentication:\n%s", text)
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

func TestBuildRuntimeConfigIncludesManualProviderHealthCheck(t *testing.T) {
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
		"  manual:\n    type: file\n    path: ./proxy_providers/manual.txt\n    health-check:\n",
		"      enable: true\n",
		"      url: \"https://cp.cloudflare.com/generate_204\"\n",
		"      interval: 300\n",
		"      timeout: 5000\n",
		"      lazy: true\n",
	} {
		if !strings.Contains(text, needle) {
			t.Fatalf("missing %q in runtime config:\n%s", needle, text)
		}
	}
}

func TestBuildServiceUnitIncludesLifecycleCommands(t *testing.T) {
	paths := Paths{
		RuntimeDir:  filepath.Join(t.TempDir(), "runtime"),
		BinPath:     filepath.Join(t.TempDir(), "bin", "minimalist"),
		ServiceUnit: filepath.Join(t.TempDir(), "systemd", "minimalist.service"),
	}
	cfg := config.Default()
	unit := BuildServiceUnit(paths, cfg)
	for _, needle := range []string{
		"ExecStartPre=+" + paths.BinPath + " apply-rules\n",
		"ExecStart=" + cfg.Install.CoreBin + " -d " + paths.RuntimeDir + "\n",
		"ExecReload=+" + paths.BinPath + " apply-rules\n",
		"ExecStopPost=+" + paths.BinPath + " clear-rules\n",
	} {
		if !strings.Contains(unit, needle) {
			t.Fatalf("missing %q in service unit:\n%s", needle, unit)
		}
	}
}

func TestBuildServiceUnitIncludesCapabilityAndPathHints(t *testing.T) {
	paths := Paths{
		RuntimeDir:  filepath.Join(t.TempDir(), "runtime"),
		BinPath:     filepath.Join(t.TempDir(), "bin", "minimalist"),
		ServiceUnit: filepath.Join(t.TempDir(), "systemd", "minimalist.service"),
	}
	cfg := config.Default()
	unit := BuildServiceUnit(paths, cfg)
	for _, needle := range []string{
		"AmbientCapabilities=CAP_NET_ADMIN CAP_NET_BIND_SERVICE CAP_NET_RAW\n",
		"CapabilityBoundingSet=CAP_NET_ADMIN CAP_NET_BIND_SERVICE CAP_NET_RAW\n",
		"ReadWritePaths=" + paths.RuntimeDir + "\n",
		"ConditionPathExists=" + paths.RuntimeConfig() + "\n",
	} {
		if !strings.Contains(unit, needle) {
			t.Fatalf("missing %q in service unit:\n%s", needle, unit)
		}
	}
}

func TestBuildSysctlDefaultsToIPv4Forwarding(t *testing.T) {
	cfg := config.Default()
	text := BuildSysctl(cfg)
	for _, needle := range []string{
		"net.ipv4.ip_forward = 1\n",
		"net.ipv4.conf.all.route_localnet = 1\n",
		"net.ipv4.conf.default.rp_filter = 2\n",
		"net.ipv4.conf.all.rp_filter = 2\n",
	} {
		if !strings.Contains(text, needle) {
			t.Fatalf("missing %q in sysctl output:\n%s", needle, text)
		}
	}
	if strings.Contains(text, "net.ipv6.conf.all.forwarding = 1\n") {
		t.Fatalf("did not expect ipv6 forwarding by default:\n%s", text)
	}
}

func TestBuildSysctlIncludesIPv6ForwardingWhenEnabled(t *testing.T) {
	cfg := config.Default()
	cfg.Network.EnableIPv6 = true
	text := BuildSysctl(cfg)
	if !strings.Contains(text, "net.ipv6.conf.all.forwarding = 1\n") {
		t.Fatalf("expected ipv6 forwarding in sysctl output:\n%s", text)
	}
}

func TestBuildRuntimeConfigUsesDirectOnlyProxyGroupWithoutProviders(t *testing.T) {
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
	if !strings.Contains(text, "proxy-groups:\n  - name: \"PROXY\"\n    type: select\n    proxies:\n      - DIRECT\n") {
		t.Fatalf("expected direct-only proxy group:\n%s", text)
	}
	if strings.Contains(text, "- name: \"AUTO\"") {
		t.Fatalf("did not expect AUTO group without providers:\n%s", text)
	}
}

func TestBuildRuntimeConfigIncludesAutoProxyGroupWithEnabledProviders(t *testing.T) {
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
		"- name: \"PROXY\"\n    type: select\n    proxies:\n      - DIRECT\n      - AUTO\n    use:\n      - manual\n",
		"- name: \"AUTO\"\n    type: url-test\n    url: \"https://cp.cloudflare.com/generate_204\"\n",
	} {
		if !strings.Contains(text, needle) {
			t.Fatalf("missing %q in runtime config:\n%s", needle, text)
		}
	}
}

func TestBuildRuntimeConfigAppendsDefaultRuleTail(t *testing.T) {
	paths := Paths{
		ConfigDir:  t.TempDir(),
		DataDir:    t.TempDir(),
		RuntimeDir: t.TempDir(),
	}
	if err := os.MkdirAll(paths.RulesDir(), 0o755); err != nil {
		t.Fatalf("mkdir rules dir: %v", err)
	}
	for _, file := range []struct {
		path string
		body string
	}{
		{paths.CustomRules(), "DOMAIN,example.com,DIRECT\n"},
		{paths.ACLRules(), "SRC-IP-CIDR,192.168.2.10/32,DIRECT\n"},
		{paths.BuiltinRules(), "GEOIP,LAN,DIRECT\n"},
	} {
		if err := os.WriteFile(file.path, []byte(file.body), 0o640); err != nil {
			t.Fatalf("write %s: %v", file.path, err)
		}
	}
	cfg := config.Default()
	text, err := buildRuntimeConfig(paths, cfg, state.Empty(), nil)
	if err != nil {
		t.Fatalf("build runtime config: %v", err)
	}
	for _, needle := range []string{
		"  - DOMAIN,example.com,DIRECT\n",
		"  - SRC-IP-CIDR,192.168.2.10/32,DIRECT\n",
		"  - GEOIP,LAN,DIRECT\n",
		"  - PROCESS-NAME,mihomo,DIRECT\n",
		"  - GEOIP,CN,DIRECT\n",
		"  - MATCH,PROXY\n",
	} {
		if !strings.Contains(text, needle) {
			t.Fatalf("missing %q in runtime config:\n%s", needle, text)
		}
	}
}

func TestBuildRuntimeConfigSkipsCommentedAndBlankRules(t *testing.T) {
	paths := Paths{
		ConfigDir:  t.TempDir(),
		DataDir:    t.TempDir(),
		RuntimeDir: t.TempDir(),
	}
	if err := os.MkdirAll(paths.RulesDir(), 0o755); err != nil {
		t.Fatalf("mkdir rules dir: %v", err)
	}
	body := "# comment\n\nDOMAIN,example.com,DIRECT\n"
	for _, path := range []string{paths.CustomRules(), paths.ACLRules(), paths.BuiltinRules()} {
		if err := os.WriteFile(path, []byte(body), 0o640); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	cfg := config.Default()
	text, err := buildRuntimeConfig(paths, cfg, state.Empty(), nil)
	if err != nil {
		t.Fatalf("build runtime config: %v", err)
	}
	if strings.Contains(text, "# comment") {
		t.Fatalf("comment should not appear in runtime config:\n%s", text)
	}
	if strings.Count(text, "  - DOMAIN,example.com,DIRECT\n") != 3 {
		t.Fatalf("expected one rendered line from each rules file:\n%s", text)
	}
}

func TestBuildRuntimeConfigUsesRuntimeUIPath(t *testing.T) {
	paths := Paths{
		ConfigDir:   t.TempDir(),
		DataDir:     t.TempDir(),
		RuntimeDir:  filepath.Join(t.TempDir(), "mihomo-runtime"),
		InstallDir:  t.TempDir(),
		BinPath:     filepath.Join(t.TempDir(), "minimalist"),
		ServiceUnit: filepath.Join(t.TempDir(), "minimalist.service"),
		SysctlPath:  filepath.Join(t.TempDir(), "99-minimalist-router.conf"),
	}
	cfg := config.Default()
	text, err := buildRuntimeConfig(paths, cfg, state.Empty(), nil)
	if err != nil {
		t.Fatalf("build runtime config: %v", err)
	}
	if !strings.Contains(text, "external-ui: "+paths.UIPath()+"\n") {
		t.Fatalf("expected runtime-specific ui path:\n%s", text)
	}
}

func TestBuildRuntimeConfigIncludesSubscriptionProviderHealthCheck(t *testing.T) {
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
		"  subscription-sub:\n    type: file\n    path: ./proxy_providers/subscriptions/sub-1.txt\n    health-check:\n",
		"      enable: true\n",
		"      url: \"https://cp.cloudflare.com/generate_204\"\n",
		"      interval: 300\n",
		"      timeout: 5000\n",
		"      lazy: true\n",
	} {
		if !strings.Contains(text, needle) {
			t.Fatalf("missing %q in runtime config:\n%s", needle, text)
		}
	}
}

func TestBuildRuntimeConfigIncludesAutoGroupSettings(t *testing.T) {
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
		"- name: \"AUTO\"\n    type: url-test\n",
		"    url: \"https://cp.cloudflare.com/generate_204\"\n",
		"    interval: 300\n",
		"    tolerance: 80\n",
		"    lazy: true\n",
	} {
		if !strings.Contains(text, needle) {
			t.Fatalf("missing %q in runtime config:\n%s", needle, text)
		}
	}
}

func TestBuildRuntimeConfigKeepsProxyGroupOrder(t *testing.T) {
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
	proxyIdx := strings.Index(text, "- name: \"PROXY\"")
	autoIdx := strings.Index(text, "- name: \"AUTO\"")
	if !(proxyIdx >= 0 && autoIdx > proxyIdx) {
		t.Fatalf("unexpected proxy group order:\n%s", text)
	}
}

func TestBuildRuntimeConfigIncludesRulesSectionHeader(t *testing.T) {
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
	if !strings.Contains(text, "\nrules:\n") {
		t.Fatalf("expected rules section header:\n%s", text)
	}
}

func TestBuildRuntimeConfigAlwaysIncludesLoopbackLANAllowedIP(t *testing.T) {
	paths := Paths{
		ConfigDir:  t.TempDir(),
		DataDir:    t.TempDir(),
		RuntimeDir: t.TempDir(),
	}
	cfg := config.Default()
	cfg.Network.LANCIDRs = nil
	text, err := buildRuntimeConfig(paths, cfg, state.Empty(), nil)
	if err != nil {
		t.Fatalf("build runtime config: %v", err)
	}
	if !strings.Contains(text, "  - 127.0.0.0/8\n") {
		t.Fatalf("expected loopback in lan-allowed-ips:\n%s", text)
	}
}

func TestBuildRuntimeConfigPreservesRulesRenderOrder(t *testing.T) {
	paths := Paths{
		ConfigDir:  t.TempDir(),
		DataDir:    t.TempDir(),
		RuntimeDir: t.TempDir(),
	}
	if err := os.MkdirAll(paths.RulesDir(), 0o755); err != nil {
		t.Fatalf("mkdir rules dir: %v", err)
	}
	for _, file := range []struct {
		path string
		body string
	}{
		{paths.CustomRules(), "DOMAIN,custom.example,DIRECT\n"},
		{paths.ACLRules(), "SRC-IP-CIDR,192.168.2.10/32,DIRECT\n"},
		{paths.BuiltinRules(), "GEOIP,LAN,DIRECT\n"},
	} {
		if err := os.WriteFile(file.path, []byte(file.body), 0o640); err != nil {
			t.Fatalf("write %s: %v", file.path, err)
		}
	}
	cfg := config.Default()
	text, err := buildRuntimeConfig(paths, cfg, state.Empty(), nil)
	if err != nil {
		t.Fatalf("build runtime config: %v", err)
	}
	customIdx := strings.Index(text, "  - DOMAIN,custom.example,DIRECT\n")
	aclIdx := strings.Index(text, "  - SRC-IP-CIDR,192.168.2.10/32,DIRECT\n")
	builtinIdx := strings.Index(text, "  - GEOIP,LAN,DIRECT\n")
	processIdx := strings.Index(text, "  - PROCESS-NAME,mihomo,DIRECT\n")
	matchIdx := strings.Index(text, "  - MATCH,PROXY\n")
	if !(customIdx >= 0 && aclIdx > customIdx && builtinIdx > aclIdx && processIdx > builtinIdx && matchIdx > processIdx) {
		t.Fatalf("unexpected rules order:\n%s", text)
	}
}

func TestBuildServiceUnitIncludesHardeningFlags(t *testing.T) {
	paths := Paths{
		RuntimeDir:  filepath.Join(t.TempDir(), "runtime"),
		BinPath:     filepath.Join(t.TempDir(), "bin", "minimalist"),
		ServiceUnit: filepath.Join(t.TempDir(), "systemd", "minimalist.service"),
	}
	cfg := config.Default()
	unit := BuildServiceUnit(paths, cfg)
	for _, needle := range []string{
		"NoNewPrivileges=true\n",
		"PrivateTmp=true\n",
		"ProtectHome=true\n",
		"ProtectSystem=full\n",
		"Restart=on-failure\n",
		"RestartSec=5\n",
	} {
		if !strings.Contains(unit, needle) {
			t.Fatalf("missing %q in service unit:\n%s", needle, unit)
		}
	}
}

func TestBuildServiceUnitIncludesBootDependenciesAndInstallTarget(t *testing.T) {
	paths := Paths{
		RuntimeDir:  filepath.Join(t.TempDir(), "runtime"),
		BinPath:     filepath.Join(t.TempDir(), "bin", "minimalist"),
		ServiceUnit: filepath.Join(t.TempDir(), "systemd", "minimalist.service"),
	}
	cfg := config.Default()
	unit := BuildServiceUnit(paths, cfg)
	for _, needle := range []string{
		"After=network-online.target docker.service\n",
		"Wants=network-online.target\n",
		"WantedBy=multi-user.target\n",
		"Type=simple\n",
		"LimitNOFILE=1048576\n",
	} {
		if !strings.Contains(unit, needle) {
			t.Fatalf("missing %q in service unit:\n%s", needle, unit)
		}
	}
}

func TestBuildServiceUnitUsesConfiguredCoreBin(t *testing.T) {
	paths := Paths{
		RuntimeDir:  filepath.Join(t.TempDir(), "runtime"),
		BinPath:     filepath.Join(t.TempDir(), "bin", "minimalist"),
		ServiceUnit: filepath.Join(t.TempDir(), "systemd", "minimalist.service"),
	}
	cfg := config.Default()
	cfg.Install.CoreBin = "/custom/bin/mihomo-core"
	unit := BuildServiceUnit(paths, cfg)
	if !strings.Contains(unit, "ExecStart=/custom/bin/mihomo-core -d "+paths.RuntimeDir+"\n") {
		t.Fatalf("expected configured core bin in service unit:\n%s", unit)
	}
}
