package runtime

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"minimalist/internal/config"
	"minimalist/internal/provider"
	"minimalist/internal/rulesrepo"
	"minimalist/internal/state"
)

type Paths struct {
	ConfigDir   string
	DataDir     string
	RuntimeDir  string
	InstallDir  string
	BinPath     string
	ServiceUnit string
	SysctlPath  string
}

func DefaultPaths() Paths {
	return Paths{
		ConfigDir:   getenv("MINIMALIST_CONFIG_DIR", "/etc/minimalist"),
		DataDir:     getenv("MINIMALIST_DATA_DIR", "/var/lib/minimalist"),
		RuntimeDir:  getenv("MINIMALIST_RUNTIME_DIR", "/var/lib/minimalist/mihomo"),
		InstallDir:  getenv("MINIMALIST_INSTALL_DIR", "/usr/local/lib/minimalist"),
		BinPath:     getenv("MINIMALIST_BIN_PATH", "/usr/local/bin/minimalist"),
		ServiceUnit: getenv("MINIMALIST_SERVICE_UNIT", "/etc/systemd/system/minimalist.service"),
		SysctlPath:  getenv("MINIMALIST_SYSCTL_PATH", "/etc/sysctl.d/99-minimalist-router.conf"),
	}
}

func (p Paths) ConfigPath() string { return filepath.Join(p.ConfigDir, "config.yaml") }
func (p Paths) StatePath() string  { return filepath.Join(p.DataDir, "state.json") }
func (p Paths) RulesRepoPath() string {
	return filepath.Join(p.ConfigDir, "rules-repo", "default", "manifest.yaml")
}
func (p Paths) ProviderDir() string     { return filepath.Join(p.RuntimeDir, "proxy_providers") }
func (p Paths) RulesDir() string        { return filepath.Join(p.RuntimeDir, "ruleset") }
func (p Paths) UIPath() string          { return filepath.Join(p.RuntimeDir, "ui") }
func (p Paths) ManualProvider() string  { return filepath.Join(p.ProviderDir(), "manual.txt") }
func (p Paths) BuiltinRules() string    { return filepath.Join(p.RulesDir(), "builtin.rules") }
func (p Paths) CustomRules() string     { return filepath.Join(p.RulesDir(), "custom.rules") }
func (p Paths) ACLRules() string        { return filepath.Join(p.RulesDir(), "acl.rules") }
func (p Paths) RuntimeConfig() string   { return filepath.Join(p.RuntimeDir, "config.yaml") }
func (p Paths) SubscriptionDir() string { return filepath.Join(p.ProviderDir(), "subscriptions") }
func (p Paths) SubscriptionFile(id string) string {
	return filepath.Join(p.SubscriptionDir(), id+".txt")
}
func (p Paths) SubscriptionRelPath(id string) string {
	return "./proxy_providers/subscriptions/" + id + ".txt"
}
func (p Paths) SubscriptionProviderName(id string) string {
	short := strings.SplitN(id, "-", 2)[0]
	if short == "" {
		short = id
	}
	return "subscription-" + short
}

func EnsureLayout(paths Paths) error {
	dirs := []string{
		paths.ConfigDir,
		paths.DataDir,
		paths.RuntimeDir,
		paths.InstallDir,
		paths.ProviderDir(),
		paths.SubscriptionDir(),
		paths.RulesDir(),
		paths.UIPath(),
		filepath.Dir(paths.ServiceUnit),
		filepath.Dir(paths.SysctlPath),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func RenderFiles(paths Paths, cfg config.Config, st state.State) error {
	if err := EnsureLayout(paths); err != nil {
		return err
	}
	if err := provider.RenderProvider(paths.ManualProvider(), st.Nodes, "", "subscription"); err != nil {
		return err
	}
	if err := writeRules(paths.CustomRules(), st.Rules); err != nil {
		return err
	}
	if err := writeRules(paths.ACLRules(), st.ACL); err != nil {
		return err
	}
	builtin, err := rulesrepo.Render(paths.RulesRepoPath())
	if err != nil {
		return err
	}
	if err := os.WriteFile(paths.BuiltinRules(), []byte(strings.Join(builtin, "\n")+"\n"), 0o640); err != nil {
		return err
	}
	content, err := buildRuntimeConfig(paths, cfg, st, builtin)
	if err != nil {
		return err
	}
	return os.WriteFile(paths.RuntimeConfig(), []byte(content), 0o640)
}

func writeRules(path string, rules []state.Rule) error {
	lines := []string{}
	kindMap := map[string]string{
		"domain":   "DOMAIN",
		"suffix":   "DOMAIN-SUFFIX",
		"keyword":  "DOMAIN-KEYWORD",
		"src-cidr": "SRC-IP-CIDR",
		"ip-cidr":  "IP-CIDR",
		"port":     "DST-PORT",
		"geoip":    "GEOIP",
		"geosite":  "GEOSITE",
		"ruleset":  "RULE-SET",
	}
	for _, rule := range rules {
		kind := kindMap[rule.Kind]
		if kind == "" {
			return fmt.Errorf("unsupported rule kind: %s", rule.Kind)
		}
		lines = append(lines, fmt.Sprintf("%s,%s,%s", kind, rule.Pattern, rule.Target))
	}
	body := ""
	if len(lines) > 0 {
		body = strings.Join(lines, "\n") + "\n"
	}
	return os.WriteFile(path, []byte(body), 0o640)
}

func buildRuntimeConfig(paths Paths, cfg config.Config, st state.State, builtin []string) (string, error) {
	var b strings.Builder
	secret := cfg.Controller.Secret
	if secret == "" {
		secret = "minimalist-secret"
	}
	fmt.Fprintf(&b, "mixed-port: %d\n", cfg.Ports.Mixed)
	fmt.Fprintf(&b, "tproxy-port: %d\n", cfg.Ports.TProxy)
	b.WriteString("allow-lan: true\n")
	b.WriteString("bind-address: \"*\"\n")
	b.WriteString("lan-allowed-ips:\n")
	for _, cidr := range append(append([]string{}, cfg.Network.LANCIDRs...), "127.0.0.0/8") {
		fmt.Fprintf(&b, "  - %s\n", cidr)
	}
	if len(cfg.Access.LANDisallowedCIDRs) > 0 {
		b.WriteString("lan-disallowed-ips:\n")
		for _, cidr := range cfg.Access.LANDisallowedCIDRs {
			fmt.Fprintf(&b, "  - %s\n", cidr)
		}
	}
	fmt.Fprintf(&b, "mode: %s\n", cfg.Profile.Mode)
	b.WriteString("log-level: info\n")
	fmt.Fprintf(&b, "ipv6: %t\n", cfg.Network.EnableIPv6)
	b.WriteString("unified-delay: true\n")
	b.WriteString("tcp-concurrent: true\n")
	b.WriteString("find-process-mode: off\n")
	b.WriteString("geodata-mode: false\n")
	b.WriteString("geo-auto-update: false\n")
	b.WriteString("geo-update-interval: 24\n")
	b.WriteString("geox-url:\n")
	b.WriteString("  mmdb: \"https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/country.mmdb\"\n")
	b.WriteString("  geoip: \"https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/geoip.dat\"\n")
	b.WriteString("  geosite: \"https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/geosite.dat\"\n")
	fmt.Fprintf(&b, "\nexternal-controller: %s:%d\n", cfg.Controller.BindAddress, cfg.Ports.Controller)
	fmt.Fprintf(&b, "secret: %q\n", secret)
	fmt.Fprintf(&b, "external-ui: %s\n", paths.UIPath())
	if len(cfg.Controller.CORSAllowOrigins) > 0 || cfg.Controller.CORSAllowPrivateNetwork {
		b.WriteString("external-controller-cors:\n")
		if len(cfg.Controller.CORSAllowOrigins) > 0 {
			b.WriteString("  allow-origins:\n")
			for _, origin := range cfg.Controller.CORSAllowOrigins {
				fmt.Fprintf(&b, "    - %q\n", origin)
			}
		}
		fmt.Fprintf(&b, "  allow-private-network: %t\n", cfg.Controller.CORSAllowPrivateNetwork)
	}
	b.WriteString("profile:\n  store-selected: true\n  store-fake-ip: true\n")
	b.WriteString("dns:\n")
	fmt.Fprintf(&b, "  enable: true\n  listen: 0.0.0.0:%d\n", cfg.Ports.DNS)
	fmt.Fprintf(&b, "  ipv6: %t\n", cfg.Network.EnableIPv6)
	b.WriteString("  use-hosts: true\n  use-system-hosts: true\n  cache-algorithm: arc\n  respect-rules: false\n  prefer-h3: false\n")
	b.WriteString("  enhanced-mode: fake-ip\n  fake-ip-range: 198.18.0.1/16\n  fake-ip-filter-mode: blacklist\n")
	b.WriteString("  fake-ip-filter:\n")
	for _, item := range []string{"*.lan", "*.local", "+.arpa", "+.stun.*.*", "localhost.ptlogin2.qq.com", "+.msftconnecttest.com", "+.msftncsi.com", "captive.apple.com", "connectivitycheck.gstatic.com"} {
		fmt.Fprintf(&b, "    - %q\n", item)
	}
	b.WriteString("  default-nameserver:\n    - 223.5.5.5\n    - 119.29.29.29\n")
	b.WriteString("  nameserver-policy:\n")
	b.WriteString("    \"geosite:private,cn\":\n      - 223.5.5.5\n      - 119.29.29.29\n      - https://dns.alidns.com/dns-query\n      - https://doh.pub/dns-query\n")
	b.WriteString("    \"+.arpa\":\n      - 223.5.5.5\n      - 119.29.29.29\n      - https://dns.alidns.com/dns-query\n      - https://doh.pub/dns-query\n")
	b.WriteString("  nameserver:\n    - https://cloudflare-dns.com/dns-query#RULES\n    - https://dns.google/dns-query#RULES\n")
	b.WriteString("  fallback: []\n  fallback-filter:\n    geoip: false\n")
	b.WriteString("  direct-nameserver:\n    - https://dns.alidns.com/dns-query\n    - https://doh.pub/dns-query\n")
	b.WriteString("  direct-nameserver-follow-policy: true\n  proxy-server-nameserver:\n    - 223.5.5.5\n    - 119.29.29.29\n")
	if len(cfg.Access.Authentication) > 0 {
		b.WriteString("authentication:\n")
		for _, item := range cfg.Access.Authentication {
			fmt.Fprintf(&b, "  - %q\n", item)
		}
		if len(cfg.Access.SkipAuthPrefixes) > 0 {
			b.WriteString("skip-auth-prefixes:\n")
			for _, item := range cfg.Access.SkipAuthPrefixes {
				fmt.Fprintf(&b, "  - %s\n", item)
			}
		}
	}
	activeProviderNames, activeSubs, manualCount := activeProviders(paths, st)
	if len(activeProviderNames) > 0 {
		b.WriteString("\nproxy-providers:\n")
		if manualCount > 0 {
			b.WriteString("  manual:\n    type: file\n    path: ./proxy_providers/manual.txt\n    health-check:\n      enable: true\n      url: \"https://cp.cloudflare.com/generate_204\"\n      interval: 300\n      timeout: 5000\n      lazy: true\n")
		}
		for _, subID := range activeSubs {
			fmt.Fprintf(&b, "  %s:\n", paths.SubscriptionProviderName(subID))
			fmt.Fprintf(&b, "    type: file\n    path: %s\n", paths.SubscriptionRelPath(subID))
			b.WriteString("    health-check:\n      enable: true\n      url: \"https://cp.cloudflare.com/generate_204\"\n      interval: 300\n      timeout: 5000\n      lazy: true\n")
		}
		b.WriteString("\nproxy-groups:\n  - name: \"PROXY\"\n    type: select\n    proxies:\n      - DIRECT\n      - AUTO\n    use:\n")
		for _, name := range activeProviderNames {
			fmt.Fprintf(&b, "      - %s\n", name)
		}
		b.WriteString("\n  - name: \"AUTO\"\n    type: url-test\n    url: \"https://cp.cloudflare.com/generate_204\"\n    interval: 300\n    tolerance: 80\n    lazy: true\n    use:\n")
		for _, name := range activeProviderNames {
			fmt.Fprintf(&b, "      - %s\n", name)
		}
	} else {
		b.WriteString("\nproxy-groups:\n  - name: \"PROXY\"\n    type: select\n    proxies:\n      - DIRECT\n")
	}
	b.WriteString("\nrules:\n")
	for _, path := range []string{paths.CustomRules(), paths.ACLRules(), paths.BuiltinRules()} {
		lines, _ := os.ReadFile(path)
		for _, line := range strings.Split(string(lines), "\n") {
			line = strings.TrimSpace(line)
			if line != "" && !strings.HasPrefix(line, "#") {
				fmt.Fprintf(&b, "  - %s\n", line)
			}
		}
	}
	for _, line := range []string{"PROCESS-NAME,mihomo,DIRECT", "GEOIP,CN,DIRECT", "MATCH,PROXY"} {
		fmt.Fprintf(&b, "  - %s\n", line)
	}
	return b.String(), nil
}

func activeProviders(paths Paths, st state.State) ([]string, []string, int) {
	names := []string{}
	subs := []string{}
	manualCount := 0
	for _, node := range st.Nodes {
		if node.Enabled && node.Source.Kind != "subscription" {
			manualCount++
		}
	}
	if manualCount > 0 {
		names = append(names, "manual")
	}
	for _, sub := range st.Subscriptions {
		if !sub.Enabled {
			continue
		}
		if info, err := os.Stat(paths.SubscriptionFile(sub.ID)); err == nil && info.Size() > 0 {
			subs = append(subs, sub.ID)
			names = append(names, paths.SubscriptionProviderName(sub.ID))
		}
	}
	return names, subs, manualCount
}

func BuildServiceUnit(paths Paths, cfg config.Config) string {
	return fmt.Sprintf(`[Unit]
Description=Minimalist Side Router
After=network-online.target docker.service
Wants=network-online.target
ConditionPathExists=%s

[Service]
Type=simple
User=root
Group=root
ExecStartPre=+%s apply-rules
ExecStart=%s -d %s
ExecReload=+%s apply-rules
ExecStopPost=+%s clear-rules
Restart=on-failure
RestartSec=5
LimitNOFILE=1048576
AmbientCapabilities=CAP_NET_ADMIN CAP_NET_BIND_SERVICE CAP_NET_RAW
CapabilityBoundingSet=CAP_NET_ADMIN CAP_NET_BIND_SERVICE CAP_NET_RAW
NoNewPrivileges=true
PrivateTmp=true
ProtectHome=true
ProtectSystem=full
ReadWritePaths=%s

[Install]
WantedBy=multi-user.target
`, paths.RuntimeConfig(), paths.BinPath, cfg.Install.CoreBin, paths.RuntimeDir, paths.BinPath, paths.BinPath, paths.RuntimeDir)
}

func BuildSysctl(cfg config.Config) string {
	line := ""
	if cfg.Network.EnableIPv6 {
		line = "net.ipv6.conf.all.forwarding = 1\n"
	}
	return "net.ipv4.ip_forward = 1\nnet.ipv4.conf.all.route_localnet = 1\nnet.ipv4.conf.default.rp_filter = 2\nnet.ipv4.conf.all.rp_filter = 2\n" + line
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
