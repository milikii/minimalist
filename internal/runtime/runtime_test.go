package runtime

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"minimalist/internal/config"
	"minimalist/internal/rulesrepo"
	"minimalist/internal/state"
)

func assertOrderedSubstrings(t *testing.T, text string, needles []string) {
	t.Helper()
	offset := 0
	for _, needle := range needles {
		idx := strings.Index(text[offset:], needle)
		if idx < 0 {
			t.Fatalf("missing ordered snippet %q in:\n%s", needle, text)
		}
		offset += idx + len(needle)
	}
}

func TestEnsureLayoutCreatesAllExpectedDirectories(t *testing.T) {
	root := t.TempDir()
	paths := Paths{
		ConfigDir:   filepath.Join(root, "etc"),
		DataDir:     filepath.Join(root, "var"),
		RuntimeDir:  filepath.Join(root, "runtime"),
		InstallDir:  filepath.Join(root, "install"),
		BinPath:     filepath.Join(root, "bin", "minimalist"),
		ServiceUnit: filepath.Join(root, "systemd", "minimalist.service"),
		SysctlPath:  filepath.Join(root, "sysctl", "99-minimalist-router.conf"),
	}
	if err := EnsureLayout(paths); err != nil {
		t.Fatalf("ensure layout: %v", err)
	}
	for _, dir := range []string{
		paths.ConfigDir,
		paths.DataDir,
		paths.RuntimeDir,
		paths.InstallDir,
		filepath.Dir(paths.BinPath),
		paths.ProviderDir(),
		paths.SubscriptionDir(),
		paths.RulesDir(),
		paths.UIPath(),
		filepath.Dir(paths.ServiceUnit),
		filepath.Dir(paths.SysctlPath),
	} {
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			t.Fatalf("expected directory %s to exist: %v", dir, err)
		}
	}
}

func TestEnsureLayoutReturnsErrorWhenExpectedDirectoryIsFile(t *testing.T) {
	root := t.TempDir()
	paths := Paths{
		ConfigDir:   filepath.Join(root, "etc"),
		DataDir:     filepath.Join(root, "var"),
		RuntimeDir:  filepath.Join(root, "runtime"),
		InstallDir:  filepath.Join(root, "install"),
		BinPath:     filepath.Join(root, "bin", "minimalist"),
		ServiceUnit: filepath.Join(root, "systemd", "minimalist.service"),
		SysctlPath:  filepath.Join(root, "sysctl", "99-minimalist-router.conf"),
	}
	if err := os.MkdirAll(paths.RuntimeDir, 0o755); err != nil {
		t.Fatalf("mkdir runtime dir: %v", err)
	}
	if err := os.WriteFile(paths.ProviderDir(), []byte("blocked"), 0o640); err != nil {
		t.Fatalf("write blocking provider dir: %v", err)
	}
	if err := EnsureLayout(paths); err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("expected layout error, got %v", err)
	}
}

func TestDefaultPathsUseEnvironmentOverrides(t *testing.T) {
	t.Setenv("MINIMALIST_CONFIG_DIR", "/custom/etc")
	t.Setenv("MINIMALIST_DATA_DIR", "/custom/var")
	t.Setenv("MINIMALIST_RUNTIME_DIR", "/custom/runtime")
	t.Setenv("MINIMALIST_INSTALL_DIR", "/custom/install")
	t.Setenv("MINIMALIST_BIN_PATH", "/custom/bin/minimalist")
	t.Setenv("MINIMALIST_SERVICE_UNIT", "/custom/systemd/minimalist.service")
	t.Setenv("MINIMALIST_SYSCTL_PATH", "/custom/sysctl/99-minimalist-router.conf")
	paths := DefaultPaths()
	for _, tc := range []struct {
		got  string
		want string
	}{
		{paths.ConfigDir, "/custom/etc"},
		{paths.DataDir, "/custom/var"},
		{paths.RuntimeDir, "/custom/runtime"},
		{paths.InstallDir, "/custom/install"},
		{paths.BinPath, "/custom/bin/minimalist"},
		{paths.ServiceUnit, "/custom/systemd/minimalist.service"},
		{paths.SysctlPath, "/custom/sysctl/99-minimalist-router.conf"},
	} {
		if tc.got != tc.want {
			t.Fatalf("expected %s, got %s", tc.want, tc.got)
		}
	}
}

func TestPathsResolveExpectedArtifacts(t *testing.T) {
	paths := Paths{
		ConfigDir:   "/etc/minimalist",
		DataDir:     "/var/lib/minimalist",
		RuntimeDir:  "/var/lib/minimalist/mihomo",
		InstallDir:  "/usr/local/lib/minimalist",
		BinPath:     "/usr/local/bin/minimalist",
		ServiceUnit: "/etc/systemd/system/minimalist.service",
		SysctlPath:  "/etc/sysctl.d/99-minimalist-router.conf",
	}
	for _, tc := range []struct {
		got  string
		want string
	}{
		{paths.ConfigPath(), "/etc/minimalist/config.yaml"},
		{paths.StatePath(), "/var/lib/minimalist/state.json"},
		{paths.RulesRepoPath(), "/etc/minimalist/rules-repo/default/manifest.yaml"},
		{paths.ProviderDir(), "/var/lib/minimalist/mihomo/proxy_providers"},
		{paths.RulesDir(), "/var/lib/minimalist/mihomo/ruleset"},
		{paths.CountryMMDBPath(), "/var/lib/minimalist/mihomo/Country.mmdb"},
		{paths.GeoSitePath(), "/var/lib/minimalist/mihomo/GeoSite.dat"},
		{paths.UIPath(), "/var/lib/minimalist/mihomo/ui"},
		{paths.ManualProvider(), "/var/lib/minimalist/mihomo/proxy_providers/manual.txt"},
		{paths.BuiltinRules(), "/var/lib/minimalist/mihomo/ruleset/builtin.rules"},
		{paths.CustomRules(), "/var/lib/minimalist/mihomo/ruleset/custom.rules"},
		{paths.ACLRules(), "/var/lib/minimalist/mihomo/ruleset/acl.rules"},
		{paths.RuntimeConfig(), "/var/lib/minimalist/mihomo/config.yaml"},
		{paths.SubscriptionDir(), "/var/lib/minimalist/mihomo/proxy_providers/subscriptions"},
		{paths.SubscriptionFile("sub-1"), "/var/lib/minimalist/mihomo/proxy_providers/subscriptions/sub-1.txt"},
		{paths.SubscriptionRelPath("sub-1"), "./proxy_providers/subscriptions/sub-1.txt"},
	} {
		if tc.got != tc.want {
			t.Fatalf("expected %s, got %s", tc.want, tc.got)
		}
	}
}

func TestMissingRuntimeAssetsReportsAbsentFilesAndDirectories(t *testing.T) {
	root := t.TempDir()
	paths := Paths{
		ConfigDir:   filepath.Join(root, "etc"),
		DataDir:     filepath.Join(root, "var"),
		RuntimeDir:  filepath.Join(root, "runtime"),
		InstallDir:  filepath.Join(root, "install"),
		BinPath:     filepath.Join(root, "bin", "minimalist"),
		ServiceUnit: filepath.Join(root, "systemd", "minimalist.service"),
		SysctlPath:  filepath.Join(root, "sysctl", "99-minimalist-router.conf"),
	}
	if err := EnsureLayout(paths); err != nil {
		t.Fatalf("ensure layout: %v", err)
	}
	if err := os.RemoveAll(paths.UIPath()); err != nil {
		t.Fatalf("remove ui dir: %v", err)
	}

	missing := MissingRuntimeAssets(paths)
	want := []string{"Country.mmdb", "GeoSite.dat", "ui/"}
	if !reflect.DeepEqual(missing, want) {
		t.Fatalf("missing assets = %#v, want %#v", missing, want)
	}
}

func TestMissingRuntimeAssetsReturnsNilWhenAssetsPresent(t *testing.T) {
	root := t.TempDir()
	paths := Paths{
		ConfigDir:   filepath.Join(root, "etc"),
		DataDir:     filepath.Join(root, "var"),
		RuntimeDir:  filepath.Join(root, "runtime"),
		InstallDir:  filepath.Join(root, "install"),
		BinPath:     filepath.Join(root, "bin", "minimalist"),
		ServiceUnit: filepath.Join(root, "systemd", "minimalist.service"),
		SysctlPath:  filepath.Join(root, "sysctl", "99-minimalist-router.conf"),
	}
	if err := EnsureLayout(paths); err != nil {
		t.Fatalf("ensure layout: %v", err)
	}
	if err := os.WriteFile(paths.CountryMMDBPath(), []byte("mmdb"), 0o640); err != nil {
		t.Fatalf("write mmdb: %v", err)
	}
	if err := os.WriteFile(paths.GeoSitePath(), []byte("geosite"), 0o640); err != nil {
		t.Fatalf("write geosite: %v", err)
	}

	missing := MissingRuntimeAssets(paths)
	if len(missing) != 0 {
		t.Fatalf("expected no missing assets, got %#v", missing)
	}
}

func TestSubscriptionProviderNameDerivesStablePrefix(t *testing.T) {
	paths := Paths{}
	for _, tc := range []struct {
		id   string
		want string
	}{
		{"sub-1", "subscription-sub"},
		{"demo", "subscription-demo"},
		{"", "subscription-"},
	} {
		if got := paths.SubscriptionProviderName(tc.id); got != tc.want {
			t.Fatalf("id=%q expected %s, got %s", tc.id, tc.want, got)
		}
	}
}

func TestSubscriptionProviderNameTrimsOnlyFirstDashSeparatedSegment(t *testing.T) {
	paths := Paths{}
	if got := paths.SubscriptionProviderName("alpha-beta-gamma"); got != "subscription-alpha" {
		t.Fatalf("expected first segment prefix, got %q", got)
	}
}

func TestSubscriptionFileAndRelPathUseSameID(t *testing.T) {
	paths := Paths{RuntimeDir: "/var/lib/minimalist/mihomo"}
	if paths.SubscriptionFile("sub-1") != "/var/lib/minimalist/mihomo/proxy_providers/subscriptions/sub-1.txt" {
		t.Fatalf("unexpected subscription file path: %s", paths.SubscriptionFile("sub-1"))
	}
	if paths.SubscriptionRelPath("sub-1") != "./proxy_providers/subscriptions/sub-1.txt" {
		t.Fatalf("unexpected subscription rel path: %s", paths.SubscriptionRelPath("sub-1"))
	}
}

func TestWriteRulesRejectsUnsupportedKind(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rules.txt")
	err := writeRules(path, []state.Rule{{Kind: "unknown", Pattern: "x", Target: "DIRECT"}})
	if err == nil || !strings.Contains(err.Error(), "unsupported rule kind: unknown") {
		t.Fatalf("expected unsupported kind error, got %v", err)
	}
}

func TestWriteRulesMapsSupportedKinds(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rules.txt")
	rules := []state.Rule{
		{Kind: "domain", Pattern: "example.com", Target: "DIRECT"},
		{Kind: "suffix", Pattern: "example.org", Target: "DIRECT"},
		{Kind: "keyword", Pattern: "google", Target: "PROXY"},
		{Kind: "src-cidr", Pattern: "192.168.2.10/32", Target: "DIRECT"},
		{Kind: "ip-cidr", Pattern: "10.0.0.0/24", Target: "DIRECT"},
		{Kind: "port", Pattern: "443", Target: "DIRECT"},
		{Kind: "geoip", Pattern: "CN", Target: "DIRECT"},
		{Kind: "geosite", Pattern: "private", Target: "DIRECT"},
		{Kind: "ruleset", Pattern: "custom", Target: "PROXY"},
	}
	if err := writeRules(path, rules); err != nil {
		t.Fatalf("write rules: %v", err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read rules file: %v", err)
	}
	text := string(body)
	for _, needle := range []string{
		"DOMAIN,example.com,DIRECT\n",
		"DOMAIN-SUFFIX,example.org,DIRECT\n",
		"DOMAIN-KEYWORD,google,PROXY\n",
		"SRC-IP-CIDR,192.168.2.10/32,DIRECT\n",
		"IP-CIDR,10.0.0.0/24,DIRECT\n",
		"DST-PORT,443,DIRECT\n",
		"GEOIP,CN,DIRECT\n",
		"GEOSITE,private,DIRECT\n",
		"RULE-SET,custom,PROXY\n",
	} {
		if !strings.Contains(text, needle) {
			t.Fatalf("missing %q in rules file:\n%s", needle, text)
		}
	}
}

func TestRenderFilesWritesAllRuntimeArtifacts(t *testing.T) {
	paths := Paths{
		ConfigDir:   filepath.Join(t.TempDir(), "etc"),
		DataDir:     filepath.Join(t.TempDir(), "var"),
		RuntimeDir:  filepath.Join(t.TempDir(), "runtime"),
		InstallDir:  filepath.Join(t.TempDir(), "install"),
		BinPath:     filepath.Join(t.TempDir(), "bin", "minimalist"),
		ServiceUnit: filepath.Join(t.TempDir(), "systemd", "minimalist.service"),
		SysctlPath:  filepath.Join(t.TempDir(), "sysctl", "99-minimalist-router.conf"),
	}
	if err := rulesrepo.InitDefaultRepo(filepath.Dir(paths.RulesRepoPath())); err != nil {
		t.Fatalf("init rules repo: %v", err)
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
	if err := RenderFiles(paths, cfg, st); err != nil {
		t.Fatalf("render files: %v", err)
	}
	for _, file := range []string{
		paths.ManualProvider(),
		paths.BuiltinRules(),
		paths.RuntimeConfig(),
		paths.CustomRules(),
		paths.ACLRules(),
	} {
		if _, err := os.Stat(file); err != nil {
			t.Fatalf("expected artifact %s: %v", file, err)
		}
	}
	body, err := os.ReadFile(paths.RuntimeConfig())
	if err != nil {
		t.Fatalf("read runtime config: %v", err)
	}
	for _, needle := range []string{"proxy-groups:", "rules:", "manual:"} {
		if !strings.Contains(string(body), needle) {
			t.Fatalf("missing %q in runtime config:\n%s", needle, string(body))
		}
	}
}

func TestRenderFilesWritesDirectOnlyRuntimeConfigWithoutActiveProviders(t *testing.T) {
	paths := Paths{
		ConfigDir:   filepath.Join(t.TempDir(), "etc"),
		DataDir:     filepath.Join(t.TempDir(), "var"),
		RuntimeDir:  filepath.Join(t.TempDir(), "runtime"),
		InstallDir:  filepath.Join(t.TempDir(), "install"),
		BinPath:     filepath.Join(t.TempDir(), "bin", "minimalist"),
		ServiceUnit: filepath.Join(t.TempDir(), "systemd", "minimalist.service"),
		SysctlPath:  filepath.Join(t.TempDir(), "sysctl", "99-minimalist-router.conf"),
	}
	if err := rulesrepo.InitDefaultRepo(filepath.Dir(paths.RulesRepoPath())); err != nil {
		t.Fatalf("init rules repo: %v", err)
	}
	cfg := config.Default()
	cfg.Controller.CORSAllowOrigins = []string{"https://panel.example"}
	cfg.Controller.CORSAllowPrivateNetwork = true
	cfg.Access.Authentication = []string{"user:pass"}
	cfg.Access.SkipAuthPrefixes = []string{"192.168.2."}
	if err := RenderFiles(paths, cfg, state.Empty()); err != nil {
		t.Fatalf("render files: %v", err)
	}
	body, err := os.ReadFile(paths.RuntimeConfig())
	if err != nil {
		t.Fatalf("read runtime config: %v", err)
	}
	text := string(body)
	for _, needle := range []string{
		"proxy-groups:\n  - name: \"PROXY\"\n    type: select\n    proxies:\n      - DIRECT\n",
		"external-controller-cors:",
		"authentication:",
		"skip-auth-prefixes:",
	} {
		if !strings.Contains(text, needle) {
			t.Fatalf("missing %q in runtime config:\n%s", needle, text)
		}
	}
	if strings.Contains(text, "- name: \"AUTO\"") || strings.Contains(text, "proxy-providers:") {
		t.Fatalf("did not expect active provider sections:\n%s", text)
	}
	manualBody, err := os.ReadFile(paths.ManualProvider())
	if err != nil {
		t.Fatalf("read manual provider: %v", err)
	}
	if !strings.Contains(string(manualBody), "proxies: []") {
		t.Fatalf("expected empty manual provider:\n%s", string(manualBody))
	}
}

func TestRenderFilesIncludesOnlyReadySubscriptionProviders(t *testing.T) {
	root := t.TempDir()
	paths := Paths{
		ConfigDir:   filepath.Join(root, "etc"),
		DataDir:     filepath.Join(root, "var"),
		RuntimeDir:  filepath.Join(root, "runtime"),
		InstallDir:  filepath.Join(root, "install"),
		BinPath:     filepath.Join(root, "bin", "minimalist"),
		ServiceUnit: filepath.Join(root, "systemd", "minimalist.service"),
		SysctlPath:  filepath.Join(root, "sysctl", "99-minimalist-router.conf"),
	}
	if err := rulesrepo.InitDefaultRepo(filepath.Dir(paths.RulesRepoPath())); err != nil {
		t.Fatalf("init rules repo: %v", err)
	}
	if err := EnsureLayout(paths); err != nil {
		t.Fatalf("ensure layout: %v", err)
	}
	st := state.Empty()
	st.Subscriptions = []state.Subscription{
		{ID: "ready-sub", Name: "ready", Enabled: true},
		{ID: "empty-sub", Name: "empty", Enabled: true},
		{ID: "disabled-sub", Name: "disabled", Enabled: false},
	}
	if err := os.WriteFile(paths.SubscriptionFile("ready-sub"), []byte("trojan://password@example.org:443?security=tls#ready\n"), 0o640); err != nil {
		t.Fatalf("write ready subscription cache: %v", err)
	}
	if err := os.WriteFile(paths.SubscriptionFile("empty-sub"), nil, 0o640); err != nil {
		t.Fatalf("write empty subscription cache: %v", err)
	}
	if err := os.WriteFile(paths.SubscriptionFile("disabled-sub"), []byte("trojan://password@example.org:443?security=tls#disabled\n"), 0o640); err != nil {
		t.Fatalf("write disabled subscription cache: %v", err)
	}
	if err := RenderFiles(paths, config.Default(), st); err != nil {
		t.Fatalf("render files: %v", err)
	}
	body, err := os.ReadFile(paths.RuntimeConfig())
	if err != nil {
		t.Fatalf("read runtime config: %v", err)
	}
	text := string(body)
	if !strings.Contains(text, "subscription-ready:\n") {
		t.Fatalf("expected ready subscription provider in runtime config:\n%s", text)
	}
	if strings.Contains(text, "subscription-empty:\n") || strings.Contains(text, "subscription-disabled:\n") {
		t.Fatalf("did not expect empty or disabled subscription providers:\n%s", text)
	}
	if !strings.Contains(text, "./proxy_providers/subscriptions/ready-sub.txt") {
		t.Fatalf("expected ready subscription path in runtime config:\n%s", text)
	}
}

func TestRenderFilesReturnsErrorForInvalidRulesRepo(t *testing.T) {
	root := t.TempDir()
	paths := Paths{
		ConfigDir:   filepath.Join(root, "etc"),
		DataDir:     filepath.Join(root, "var"),
		RuntimeDir:  filepath.Join(root, "runtime"),
		InstallDir:  filepath.Join(root, "install"),
		BinPath:     filepath.Join(root, "bin", "minimalist"),
		ServiceUnit: filepath.Join(root, "systemd", "minimalist.service"),
		SysctlPath:  filepath.Join(root, "sysctl", "99-minimalist-router.conf"),
	}
	if err := EnsureLayout(paths); err != nil {
		t.Fatalf("ensure layout: %v", err)
	}
	manifest := paths.RulesRepoPath()
	if err := os.MkdirAll(filepath.Dir(manifest), 0o755); err != nil {
		t.Fatalf("make rules repo dir: %v", err)
	}
	if err := os.WriteFile(manifest, []byte("rulesets: [\n"), 0o640); err != nil {
		t.Fatalf("write invalid manifest: %v", err)
	}
	if err := RenderFiles(paths, config.Default(), state.Empty()); err == nil || !strings.Contains(err.Error(), "parse manifest") {
		t.Fatalf("expected render files to fail for invalid rules repo, got %v", err)
	}
}

func TestRenderFilesFailsWhenManualProviderPathIsDirectory(t *testing.T) {
	root := t.TempDir()
	paths := Paths{
		ConfigDir:   filepath.Join(root, "etc"),
		DataDir:     filepath.Join(root, "var"),
		RuntimeDir:  filepath.Join(root, "runtime"),
		InstallDir:  filepath.Join(root, "install"),
		BinPath:     filepath.Join(root, "bin", "minimalist"),
		ServiceUnit: filepath.Join(root, "systemd", "minimalist.service"),
		SysctlPath:  filepath.Join(root, "sysctl", "99-minimalist-router.conf"),
	}
	if err := rulesrepo.InitDefaultRepo(filepath.Dir(paths.RulesRepoPath())); err != nil {
		t.Fatalf("init rules repo: %v", err)
	}
	if err := EnsureLayout(paths); err != nil {
		t.Fatalf("ensure layout: %v", err)
	}
	if err := os.MkdirAll(paths.ManualProvider(), 0o755); err != nil {
		t.Fatalf("mkdir blocking manual provider path: %v", err)
	}
	if err := RenderFiles(paths, config.Default(), state.Empty()); err == nil || !strings.Contains(err.Error(), "is a directory") {
		t.Fatalf("expected manual provider write failure, got %v", err)
	}
}

func TestRenderFilesFailsWhenCustomRulesPathIsDirectory(t *testing.T) {
	root := t.TempDir()
	paths := Paths{
		ConfigDir:   filepath.Join(root, "etc"),
		DataDir:     filepath.Join(root, "var"),
		RuntimeDir:  filepath.Join(root, "runtime"),
		InstallDir:  filepath.Join(root, "install"),
		BinPath:     filepath.Join(root, "bin", "minimalist"),
		ServiceUnit: filepath.Join(root, "systemd", "minimalist.service"),
		SysctlPath:  filepath.Join(root, "sysctl", "99-minimalist-router.conf"),
	}
	if err := rulesrepo.InitDefaultRepo(filepath.Dir(paths.RulesRepoPath())); err != nil {
		t.Fatalf("init rules repo: %v", err)
	}
	if err := EnsureLayout(paths); err != nil {
		t.Fatalf("ensure layout: %v", err)
	}
	if err := os.MkdirAll(paths.CustomRules(), 0o755); err != nil {
		t.Fatalf("mkdir blocking custom rules path: %v", err)
	}
	if err := RenderFiles(paths, config.Default(), state.Empty()); err == nil || !strings.Contains(err.Error(), "is a directory") {
		t.Fatalf("expected custom rules write failure, got %v", err)
	}
}

func TestRenderFilesFailsWhenACLRulesPathIsDirectory(t *testing.T) {
	root := t.TempDir()
	paths := Paths{
		ConfigDir:   filepath.Join(root, "etc"),
		DataDir:     filepath.Join(root, "var"),
		RuntimeDir:  filepath.Join(root, "runtime"),
		InstallDir:  filepath.Join(root, "install"),
		BinPath:     filepath.Join(root, "bin", "minimalist"),
		ServiceUnit: filepath.Join(root, "systemd", "minimalist.service"),
		SysctlPath:  filepath.Join(root, "sysctl", "99-minimalist-router.conf"),
	}
	if err := rulesrepo.InitDefaultRepo(filepath.Dir(paths.RulesRepoPath())); err != nil {
		t.Fatalf("init rules repo: %v", err)
	}
	if err := EnsureLayout(paths); err != nil {
		t.Fatalf("ensure layout: %v", err)
	}
	if err := os.MkdirAll(paths.ACLRules(), 0o755); err != nil {
		t.Fatalf("mkdir blocking acl rules path: %v", err)
	}
	if err := RenderFiles(paths, config.Default(), state.Empty()); err == nil || !strings.Contains(err.Error(), "is a directory") {
		t.Fatalf("expected acl rules write failure, got %v", err)
	}
}

func TestRenderFilesFailsWhenRuntimeConfigPathIsDirectory(t *testing.T) {
	root := t.TempDir()
	paths := Paths{
		ConfigDir:   filepath.Join(root, "etc"),
		DataDir:     filepath.Join(root, "var"),
		RuntimeDir:  filepath.Join(root, "runtime"),
		InstallDir:  filepath.Join(root, "install"),
		BinPath:     filepath.Join(root, "bin", "minimalist"),
		ServiceUnit: filepath.Join(root, "systemd", "minimalist.service"),
		SysctlPath:  filepath.Join(root, "sysctl", "99-minimalist-router.conf"),
	}
	if err := rulesrepo.InitDefaultRepo(filepath.Dir(paths.RulesRepoPath())); err != nil {
		t.Fatalf("init rules repo: %v", err)
	}
	if err := EnsureLayout(paths); err != nil {
		t.Fatalf("ensure layout: %v", err)
	}
	if err := os.MkdirAll(paths.RuntimeConfig(), 0o755); err != nil {
		t.Fatalf("mkdir blocking runtime config path: %v", err)
	}
	if err := RenderFiles(paths, config.Default(), state.Empty()); err == nil || !strings.Contains(err.Error(), "is a directory") {
		t.Fatalf("expected runtime config write failure, got %v", err)
	}
}

func TestRenderFilesFailsWhenBuiltinRulesPathIsDirectory(t *testing.T) {
	root := t.TempDir()
	paths := Paths{
		ConfigDir:   filepath.Join(root, "etc"),
		DataDir:     filepath.Join(root, "var"),
		RuntimeDir:  filepath.Join(root, "runtime"),
		InstallDir:  filepath.Join(root, "install"),
		BinPath:     filepath.Join(root, "bin", "minimalist"),
		ServiceUnit: filepath.Join(root, "systemd", "minimalist.service"),
		SysctlPath:  filepath.Join(root, "sysctl", "99-minimalist-router.conf"),
	}
	if err := rulesrepo.InitDefaultRepo(filepath.Dir(paths.RulesRepoPath())); err != nil {
		t.Fatalf("init rules repo: %v", err)
	}
	if err := EnsureLayout(paths); err != nil {
		t.Fatalf("ensure layout: %v", err)
	}
	if err := os.MkdirAll(paths.BuiltinRules(), 0o755); err != nil {
		t.Fatalf("mkdir blocking builtin rules path: %v", err)
	}
	if err := RenderFiles(paths, config.Default(), state.Empty()); err == nil || !strings.Contains(err.Error(), "is a directory") {
		t.Fatalf("expected builtin rules write failure, got %v", err)
	}
}

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

func TestBuildRuntimeConfigIncludesFullFakeIPFilterList(t *testing.T) {
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
		`    - "*.lan"`,
		`    - "*.local"`,
		`    - "+.arpa"`,
		`    - "+.stun.*.*"`,
		`    - "localhost.ptlogin2.qq.com"`,
		`    - "+.msftconnecttest.com"`,
		`    - "+.msftncsi.com"`,
		`    - "captive.apple.com"`,
		`    - "connectivitycheck.gstatic.com"`,
	} {
		if !strings.Contains(text, needle) {
			t.Fatalf("missing %q in runtime config:\n%s", needle, text)
		}
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

func TestBuildRuntimeConfigLocksMatureDNSBaselineContract(t *testing.T) {
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

	assertOrderedSubstrings(t, text, []string{
		"dns:\n",
		"  enable: true\n",
		"  listen: 0.0.0.0:1053\n",
		"  use-hosts: true\n",
		"  use-system-hosts: true\n",
		"  cache-algorithm: arc\n",
		"  respect-rules: false\n",
		"  prefer-h3: false\n",
		"  enhanced-mode: fake-ip\n",
		"  fake-ip-range: 198.18.0.1/16\n",
		"  fake-ip-filter-mode: blacklist\n",
		"  default-nameserver:\n",
		"    - 223.5.5.5\n",
		"    - 119.29.29.29\n",
		"  nameserver-policy:\n",
		"    \"geosite:private,cn\":\n      - 223.5.5.5\n      - 119.29.29.29\n      - https://dns.alidns.com/dns-query\n      - https://doh.pub/dns-query\n",
		"    \"+.arpa\":\n      - 223.5.5.5\n      - 119.29.29.29\n      - https://dns.alidns.com/dns-query\n      - https://doh.pub/dns-query\n",
		"  nameserver:\n",
		"    - https://cloudflare-dns.com/dns-query#RULES\n",
		"    - https://dns.google/dns-query#RULES\n",
		"  fallback: []\n",
		"  fallback-filter:\n",
		"    geoip: false\n",
		"  direct-nameserver:\n",
		"    - https://dns.alidns.com/dns-query\n",
		"    - https://doh.pub/dns-query\n",
		"  direct-nameserver-follow-policy: true\n",
		"  proxy-server-nameserver:\n",
		"    - 223.5.5.5\n",
		"    - 119.29.29.29\n",
	})

	for _, needle := range []string{
		`    - "*.lan"`,
		`    - "*.local"`,
		`    - "+.arpa"`,
		`    - "+.stun.*.*"`,
	} {
		if !strings.Contains(text, needle) {
			t.Fatalf("missing %q in runtime config:\n%s", needle, text)
		}
	}
}

func TestBuildRuntimeConfigKeepsNameserverPolicyAndFallbackOrdering(t *testing.T) {
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
	nameserverIdx := strings.Index(text, "  nameserver:\n")
	fallbackIdx := strings.Index(text, "  fallback: []\n")
	if !(nameserverIdx >= 0 && fallbackIdx > nameserverIdx) {
		t.Fatalf("expected nameserver before fallback:\n%s", text)
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

func TestBuildRuntimeConfigKeepsRuntimeAssetsLocal(t *testing.T) {
	paths := Paths{
		ConfigDir:  t.TempDir(),
		DataDir:    t.TempDir(),
		RuntimeDir: filepath.Join(t.TempDir(), "mihomo"),
	}
	cfg := config.Default()
	text, err := buildRuntimeConfig(paths, cfg, state.Empty(), nil)
	if err != nil {
		t.Fatalf("build runtime config: %v", err)
	}
	for _, needle := range []string{
		"external-ui: " + paths.UIPath() + "\n",
		"geo-auto-update: false\n",
		"geodata-mode: false\n",
	} {
		if !strings.Contains(text, needle) {
			t.Fatalf("missing local asset guard %q in runtime config:\n%s", needle, text)
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

func TestBuildRuntimeConfigOmitsSubscriptionProviderWhenCacheMissing(t *testing.T) {
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
	text, err := buildRuntimeConfig(paths, cfg, st, nil)
	if err != nil {
		t.Fatalf("build runtime config: %v", err)
	}
	if strings.Contains(text, "subscription-sub") {
		t.Fatalf("did not expect subscription provider without cache:\n%s", text)
	}
}

func TestBuildRuntimeConfigOmitsSubscriptionProviderWhenCacheEmpty(t *testing.T) {
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
	if err := os.MkdirAll(paths.SubscriptionDir(), 0o755); err != nil {
		t.Fatalf("mkdir subscription dir: %v", err)
	}
	if err := os.WriteFile(paths.SubscriptionFile("sub-1"), nil, 0o640); err != nil {
		t.Fatalf("write empty subscription cache: %v", err)
	}
	text, err := buildRuntimeConfig(paths, cfg, st, nil)
	if err != nil {
		t.Fatalf("build runtime config: %v", err)
	}
	if strings.Contains(text, "subscription-sub") {
		t.Fatalf("did not expect subscription provider with empty cache:\n%s", text)
	}
}

func TestBuildRuntimeConfigOmitsDisabledSubscriptionProviderWithCache(t *testing.T) {
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
		Enabled:   false,
		CreatedAt: state.NowISO(),
	}}
	if err := os.MkdirAll(paths.SubscriptionDir(), 0o755); err != nil {
		t.Fatalf("mkdir subscription dir: %v", err)
	}
	if err := os.WriteFile(paths.SubscriptionFile("sub-1"), []byte("trojan://password@example.org:443#sub-node\n"), 0o640); err != nil {
		t.Fatalf("write subscription cache: %v", err)
	}
	text, err := buildRuntimeConfig(paths, cfg, st, nil)
	if err != nil {
		t.Fatalf("build runtime config: %v", err)
	}
	if strings.Contains(text, "subscription-sub") {
		t.Fatalf("did not expect disabled subscription provider:\n%s", text)
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
		"proxy-groups:\n  - name: \"PROXY\"\n    type: select\n    proxies:\n      - AUTO\n      - DIRECT\n    use:\n      - manual\n",
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
		"ExecStartPre=+" + paths.BinPath + " verify-runtime-assets\n",
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
		"- name: \"PROXY\"\n    type: select\n    proxies:\n      - AUTO\n      - DIRECT\n    use:\n      - manual\n",
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

func TestBuildRuntimeConfigReturnsRuleReadErrors(t *testing.T) {
	paths := Paths{
		ConfigDir:  t.TempDir(),
		DataDir:    t.TempDir(),
		RuntimeDir: t.TempDir(),
	}
	if err := os.MkdirAll(paths.CustomRules(), 0o755); err != nil {
		t.Fatalf("mkdir blocking custom rules path: %v", err)
	}
	_, err := buildRuntimeConfig(paths, config.Default(), state.Empty(), nil)
	if err == nil || !strings.Contains(err.Error(), "is a directory") {
		t.Fatalf("expected custom rules read error, got %v", err)
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

func TestBuildRuntimeConfigKeepsProxyGroupHealthCheckURL(t *testing.T) {
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
	if !strings.Contains(text, "url: \"https://cp.cloudflare.com/generate_204\"\n") {
		t.Fatalf("expected proxy-group health-check url:\n%s", text)
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

func TestBuildRuntimeConfigReturnsRuleReadError(t *testing.T) {
	root := t.TempDir()
	paths := Paths{
		ConfigDir:   filepath.Join(root, "etc"),
		DataDir:     filepath.Join(root, "var"),
		RuntimeDir:  filepath.Join(root, "runtime"),
		InstallDir:  filepath.Join(root, "install"),
		BinPath:     filepath.Join(root, "bin", "minimalist"),
		ServiceUnit: filepath.Join(root, "systemd", "minimalist.service"),
		SysctlPath:  filepath.Join(root, "sysctl", "99-minimalist-router.conf"),
	}
	if err := os.MkdirAll(paths.CustomRules(), 0o755); err != nil {
		t.Fatalf("mkdir custom rules blocker: %v", err)
	}
	cfg := config.Default()
	_, err := buildRuntimeConfig(paths, cfg, state.Empty(), nil)
	if err == nil || !strings.Contains(err.Error(), "is a directory") {
		t.Fatalf("expected rule read error, got %v", err)
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

func TestBuildServiceUnitUsesRestartPolicyAndNOFILELimit(t *testing.T) {
	paths := Paths{
		RuntimeDir:  filepath.Join(t.TempDir(), "runtime"),
		BinPath:     filepath.Join(t.TempDir(), "bin", "minimalist"),
		ServiceUnit: filepath.Join(t.TempDir(), "systemd", "minimalist.service"),
	}
	cfg := config.Default()
	unit := BuildServiceUnit(paths, cfg)
	for _, needle := range []string{
		"Restart=on-failure\n",
		"RestartSec=5\n",
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

func TestActiveProvidersIgnoresDisabledAndEmptySubscriptionCaches(t *testing.T) {
	paths := Paths{RuntimeDir: t.TempDir()}
	if err := os.MkdirAll(paths.SubscriptionDir(), 0o755); err != nil {
		t.Fatalf("mkdir subscription dir: %v", err)
	}
	if err := os.WriteFile(paths.SubscriptionFile("sub-enabled"), []byte(""), 0o640); err != nil {
		t.Fatalf("write empty subscription cache: %v", err)
	}
	if err := os.WriteFile(paths.SubscriptionFile("sub-ready"), []byte("trojan://secret@example.com:443\n"), 0o640); err != nil {
		t.Fatalf("write non-empty subscription cache: %v", err)
	}

	st := state.Empty()
	st.Nodes = []state.Node{
		{Enabled: true, Source: state.Source{Kind: "manual"}},
		{Enabled: false, Source: state.Source{Kind: "manual"}},
	}
	st.Subscriptions = []state.Subscription{
		{ID: "sub-enabled", Enabled: true},
		{ID: "sub-disabled", Enabled: false},
		{ID: "sub-ready", Enabled: true},
	}

	names, subs, manualCount := activeProviders(paths, st)
	if manualCount != 1 {
		t.Fatalf("expected one active manual provider, got %d", manualCount)
	}
	if len(names) != 2 || names[0] != "manual" || names[1] != "subscription-sub" {
		t.Fatalf("unexpected provider names: %#v", names)
	}
	if len(subs) != 1 || subs[0] != "sub-ready" {
		t.Fatalf("unexpected active subscriptions: %#v", subs)
	}
}

func TestActiveProvidersIgnoresUnsupportedSubscriptionCaches(t *testing.T) {
	paths := Paths{RuntimeDir: t.TempDir()}
	if err := os.MkdirAll(paths.SubscriptionDir(), 0o755); err != nil {
		t.Fatalf("mkdir subscription dir: %v", err)
	}
	if err := os.WriteFile(paths.SubscriptionFile("sub-unsupported"), []byte("ssh://unsupported.example.com\n"), 0o640); err != nil {
		t.Fatalf("write unsupported subscription cache: %v", err)
	}

	st := state.Empty()
	st.Subscriptions = []state.Subscription{{ID: "sub-unsupported", Enabled: true}}

	names, subs, manualCount := activeProviders(paths, st)
	if manualCount != 0 {
		t.Fatalf("expected no active manual provider, got %d", manualCount)
	}
	if len(names) != 0 || len(subs) != 0 {
		t.Fatalf("expected unsupported subscription cache to be ignored, got names=%#v subs=%#v", names, subs)
	}
}

func TestGetenvFallsBackOnEmptyValue(t *testing.T) {
	t.Setenv("MINIMALIST_TEST_ENV", "")
	if got := getenv("MINIMALIST_TEST_ENV", "fallback"); got != "fallback" {
		t.Fatalf("expected fallback, got %q", got)
	}
	t.Setenv("MINIMALIST_TEST_ENV", "configured")
	if got := getenv("MINIMALIST_TEST_ENV", "fallback"); got != "configured" {
		t.Fatalf("expected configured env, got %q", got)
	}
}

func TestRenderFilesRejectsUnsupportedRuleKinds(t *testing.T) {
	paths := Paths{
		ConfigDir:   filepath.Join(t.TempDir(), "etc"),
		DataDir:     filepath.Join(t.TempDir(), "var"),
		RuntimeDir:  filepath.Join(t.TempDir(), "runtime"),
		InstallDir:  filepath.Join(t.TempDir(), "install"),
		BinPath:     filepath.Join(t.TempDir(), "bin", "minimalist"),
		ServiceUnit: filepath.Join(t.TempDir(), "systemd", "minimalist.service"),
		SysctlPath:  filepath.Join(t.TempDir(), "sysctl", "99-minimalist-router.conf"),
	}
	if err := rulesrepo.InitDefaultRepo(filepath.Dir(paths.RulesRepoPath())); err != nil {
		t.Fatalf("init rules repo: %v", err)
	}
	cfg := config.Default()
	st := state.Empty()
	st.Rules = []state.Rule{{Kind: "unknown", Pattern: "example.com", Target: "DIRECT"}}
	if err := RenderFiles(paths, cfg, st); err == nil || !strings.Contains(err.Error(), "unsupported rule kind: unknown") {
		t.Fatalf("expected unsupported rule kind error, got %v", err)
	}
}
