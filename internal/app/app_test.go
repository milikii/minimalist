package app

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"minimalist/internal/config"
	"minimalist/internal/rulesrepo"
	"minimalist/internal/runtime"
	"minimalist/internal/state"
)

type commandCall struct {
	name string
	args []string
}

type fakeRunner struct {
	runFn    func(name string, args ...string) error
	outputFn func(name string, args ...string) (string, string, error)
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type errorReadCloser struct {
	err error
}

func (e errorReadCloser) Read([]byte) (int, error) {
	return 0, e.err
}

func (e errorReadCloser) Close() error {
	return nil
}

func textResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}

func TestNewInitializesDefaultDependencies(t *testing.T) {
	app := New()
	if app.Runner == nil {
		t.Fatalf("expected runner to be initialized")
	}
	if app.Client == nil {
		t.Fatalf("expected client to be initialized")
	}
	if app.Stdout == nil || app.Stderr == nil || app.Stdin == nil {
		t.Fatalf("expected stdio to be initialized")
	}
	if app.Paths.ConfigDir == "" || app.Paths.DataDir == "" || app.Paths.RuntimeDir == "" {
		t.Fatalf("expected default paths to be initialized: %+v", app.Paths)
	}
}

func TestRequireRootReturnsErrorWhenNotRoot(t *testing.T) {
	app, _ := newTestApp(t)
	oldGeteuid := geteuid
	geteuid = func() int { return 1000 }
	defer func() { geteuid = oldGeteuid }()

	err := app.requireRoot()
	if err == nil || !strings.Contains(err.Error(), "请用 root 运行") {
		t.Fatalf("expected root error, got %v", err)
	}
}

func TestInstallSelfCopiesBinaryAndInitializesAssets(t *testing.T) {
	app, _ := newTestApp(t)
	if err := app.InstallSelf(); err != nil {
		t.Fatalf("install self: %v", err)
	}
	for _, path := range []string{
		app.Paths.BinPath,
		app.Paths.ConfigPath(),
		app.Paths.StatePath(),
		app.Paths.RulesRepoPath(),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected %s to exist: %v", path, err)
		}
	}
	if _, err := os.ReadFile(app.Paths.BinPath); err != nil {
		t.Fatalf("expected copied binary to be readable: %v", err)
	}
}

func TestInstallSelfFailsWhenLayoutCannotBeEnsured(t *testing.T) {
	app, _ := newTestApp(t)
	if os.Geteuid() != 0 {
		t.Skip("root required")
	}
	if err := os.WriteFile(app.Paths.ConfigDir, []byte("blocked"), 0o640); err != nil {
		t.Fatalf("write blocking file: %v", err)
	}
	err := app.InstallSelf()
	if err == nil || (!strings.Contains(err.Error(), "not a directory") && !strings.Contains(err.Error(), "file exists")) {
		t.Fatalf("expected layout failure, got %v", err)
	}
}

func TestInstallSelfFailsWhenBinaryPathIsDirectory(t *testing.T) {
	app, _ := newTestApp(t)
	if os.Geteuid() != 0 {
		t.Skip("root required")
	}
	if err := os.MkdirAll(app.Paths.BinPath, 0o755); err != nil {
		t.Fatalf("mkdir blocking directory: %v", err)
	}
	err := app.InstallSelf()
	if err == nil || !strings.Contains(err.Error(), "is a directory") {
		t.Fatalf("expected binary write failure, got %v", err)
	}
}

func TestInstallSelfFailsWhenRulesRepoParentIsFile(t *testing.T) {
	app, _ := newTestApp(t)
	oldGeteuid := geteuid
	geteuid = func() int { return 0 }
	defer func() { geteuid = oldGeteuid }()

	if err := os.MkdirAll(app.Paths.ConfigDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	blocked := filepath.Join(app.Paths.ConfigDir, "rules-repo")
	if err := os.WriteFile(blocked, []byte("blocked"), 0o640); err != nil {
		t.Fatalf("write blocking rules repo parent: %v", err)
	}
	err := app.InstallSelf()
	if err == nil || (!strings.Contains(err.Error(), "not a directory") && !strings.Contains(err.Error(), "file exists")) {
		t.Fatalf("expected rules repo init failure, got %v", err)
	}
}

func TestInstallSelfFailsWhenStatePathIsDirectory(t *testing.T) {
	app, _ := newTestApp(t)
	oldGeteuid := geteuid
	geteuid = func() int { return 0 }
	defer func() { geteuid = oldGeteuid }()

	if err := os.Remove(app.Paths.StatePath()); err != nil && !os.IsNotExist(err) {
		t.Fatalf("remove state file: %v", err)
	}
	if err := os.MkdirAll(app.Paths.StatePath(), 0o755); err != nil {
		t.Fatalf("mkdir blocking state path: %v", err)
	}
	err := app.InstallSelf()
	if err == nil || !strings.Contains(err.Error(), "is a directory") {
		t.Fatalf("expected state write failure, got %v", err)
	}
}

func TestInstallSelfReturnsRootErrorWhenNotRoot(t *testing.T) {
	app, _ := newTestApp(t)
	oldGeteuid := geteuid
	geteuid = func() int { return 1000 }
	defer func() { geteuid = oldGeteuid }()

	if err := app.InstallSelf(); err == nil || !strings.Contains(err.Error(), "请用 root 运行") {
		t.Fatalf("expected root error, got %v", err)
	}
}

func TestSetupFailsWhenConfigPathIsDirectory(t *testing.T) {
	app, _ := newTestApp(t)
	if os.Geteuid() != 0 {
		t.Skip("root required")
	}
	if err := os.MkdirAll(app.Paths.ConfigPath(), 0o755); err != nil {
		t.Fatalf("mkdir blocking config path: %v", err)
	}
	err := app.Setup()
	if err == nil || !strings.Contains(err.Error(), "is a directory") {
		t.Fatalf("expected config load failure, got %v", err)
	}
}

func TestSetupFailsWhenStatePathIsDirectory(t *testing.T) {
	app, _ := newTestApp(t)
	if os.Geteuid() != 0 {
		t.Skip("root required")
	}
	if err := os.MkdirAll(app.Paths.StatePath(), 0o755); err != nil {
		t.Fatalf("mkdir blocking state path: %v", err)
	}
	err := app.Setup()
	if err == nil || !strings.Contains(err.Error(), "is a directory") {
		t.Fatalf("expected state load failure, got %v", err)
	}
}

func TestNormalizeRuleHelpersCoverLegacyAliases(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want string
	}{
		{"domain_suffix", "suffix"},
		{"domain-keyword", "keyword"},
		{"src", "src-cidr"},
		{"dst", "ip-cidr"},
		{"rule-set", "ruleset"},
	} {
		if got := normalizeRuleInput(tc.in); got != tc.want {
			t.Fatalf("normalizeRuleInput(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
	for _, tc := range []struct {
		in   string
		want string
	}{
		{"domain", "DOMAIN"},
		{"suffix", "DOMAIN-SUFFIX"},
		{"keyword", "DOMAIN-KEYWORD"},
		{"src-cidr", "SRC-IP-CIDR"},
		{"ip-cidr", "IP-CIDR"},
		{"ruleset", "RULE-SET"},
	} {
		if got := normalizeRuleKind(tc.in); got != tc.want {
			t.Fatalf("normalizeRuleKind(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestAppendIfMissingRuleAndTerminalHelpers(t *testing.T) {
	rules := []state.Rule{{ID: "1", Kind: "domain", Pattern: "example.com", Target: "DIRECT"}}
	rules = appendIfMissingRule(rules, state.Rule{ID: "2", Kind: "domain", Pattern: "example.com", Target: "DIRECT"})
	if len(rules) != 1 {
		t.Fatalf("expected duplicate rule to be skipped, got %#v", rules)
	}
	rules = appendIfMissingRule(rules, state.Rule{ID: "3", Kind: "domain", Pattern: "example.org", Target: "DIRECT"})
	if len(rules) != 2 {
		t.Fatalf("expected new rule to be appended, got %#v", rules)
	}
	if isTerminal(strings.NewReader("nope")) {
		t.Fatalf("expected non-file reader to be non-terminal")
	}
	tmp := t.TempDir()
	file, err := os.Create(filepath.Join(tmp, "input.txt"))
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer file.Close()
	if isTerminal(file) {
		t.Fatalf("expected regular file to be non-terminal")
	}
	if isCharDevice(file) {
		t.Fatalf("expected regular file to not be a char device")
	}
}

func TestTerminalHelpersReturnFalseForClosedFiles(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "stdin-*")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close temp file: %v", err)
	}
	if isTerminal(file) {
		t.Fatalf("expected closed file to be non-terminal")
	}
	if isCharDevice(file) {
		t.Fatalf("expected closed file to not be a char device")
	}
}

func TestHasReadyProvidersAndHTTPClientFallback(t *testing.T) {
	app, _ := newTestApp(t)
	st := state.Empty()
	if app.hasReadyProviders(st) {
		t.Fatalf("expected no ready providers in empty state")
	}
	st.Subscriptions = []state.Subscription{{ID: "sub-1", Enabled: true}}
	if app.hasReadyProviders(st) {
		t.Fatalf("expected no ready providers without cache file")
	}
	if err := os.MkdirAll(app.Paths.SubscriptionDir(), 0o755); err != nil {
		t.Fatalf("mkdir subscription dir: %v", err)
	}
	if err := os.WriteFile(app.Paths.SubscriptionFile("sub-1"), []byte("trojan://password@example.org:443\n"), 0o640); err != nil {
		t.Fatalf("write subscription cache: %v", err)
	}
	if !app.hasReadyProviders(st) {
		t.Fatalf("expected ready providers with subscription cache")
	}
	app.Client = nil
	if client := app.httpClient(); client == nil {
		t.Fatalf("expected fallback http client")
	}
}

func TestRemoveCommandsRejectOutOfRangeIndexes(t *testing.T) {
	app, _ := newTestApp(t)
	if err := app.RemoveNode(1); err == nil || !strings.Contains(err.Error(), "node index out of range") {
		t.Fatalf("expected node range error, got %v", err)
	}
	if err := app.RemoveRule(false, 1); err == nil || !strings.Contains(err.Error(), "rule index out of range") {
		t.Fatalf("expected rule range error, got %v", err)
	}
	if err := app.RemoveRule(true, 1); err == nil || !strings.Contains(err.Error(), "rule index out of range") {
		t.Fatalf("expected acl range error, got %v", err)
	}
	if err := app.RemoveSubscription(1); err == nil || !strings.Contains(err.Error(), "subscription index out of range") {
		t.Fatalf("expected subscription range error, got %v", err)
	}
}

func TestReadOnlyCommandsReturnEnsureAllErrors(t *testing.T) {
	app, _ := newTestApp(t)
	if err := os.WriteFile(app.Paths.ConfigDir, []byte("blocked"), 0o640); err != nil {
		t.Fatalf("write blocking config dir: %v", err)
	}
	tests := []struct {
		name string
		run  func() error
	}{
		{"list-nodes", app.ListNodes},
		{"list-rules", func() error { return app.ListRules(false) }},
		{"list-acl", func() error { return app.ListRules(true) }},
		{"list-subscriptions", app.ListSubscriptions},
		{"show-secret", app.ShowSecret},
		{"healthcheck", app.Healthcheck},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.run(); err == nil {
				t.Fatalf("expected ensureAll error")
			}
		})
	}
}

func TestStartRestartAndStopPropagateSystemctlErrors(t *testing.T) {
	prepare := func(t *testing.T) *App {
		t.Helper()
		app, _ := newTestApp(t)
		app.Stdin = strings.NewReader("trojan://password@example.org:443?security=tls#service-node\n")
		if err := app.ImportLinks(); err != nil {
			t.Fatalf("import links: %v", err)
		}
		if err := app.SetNodeEnabled(1, true); err != nil {
			t.Fatalf("enable node: %v", err)
		}
		return app
	}

	startApp := prepare(t)
	startApp.Runner = fakeRunner{runFn: func(name string, args ...string) error {
		if name == "systemctl" && len(args) >= 2 && args[0] == "enable" {
			return errors.New("enable failed")
		}
		return nil
	}}
	if err := startApp.Start(); err == nil || !strings.Contains(err.Error(), "enable failed") {
		t.Fatalf("expected start error, got %v", err)
	}

	restartApp := prepare(t)
	restartApp.Runner = fakeRunner{runFn: func(name string, args ...string) error {
		if name == "systemctl" && len(args) >= 1 && args[0] == "restart" {
			return errors.New("restart failed")
		}
		return nil
	}}
	if err := restartApp.Restart(); err == nil || !strings.Contains(err.Error(), "restart failed") {
		t.Fatalf("expected restart error, got %v", err)
	}

	stopApp, _ := newTestApp(t)
	stopApp.Runner = fakeRunner{runFn: func(name string, args ...string) error {
		return errors.New("stop failed")
	}}
	if err := stopApp.Stop(); err == nil || !strings.Contains(err.Error(), "stop failed") {
		t.Fatalf("expected stop error, got %v", err)
	}
}

func (f fakeRunner) Run(name string, args ...string) error {
	if f.runFn != nil {
		return f.runFn(name, args...)
	}
	return nil
}

func (f fakeRunner) Output(name string, args ...string) (string, string, error) {
	if f.outputFn != nil {
		return f.outputFn(name, args...)
	}
	return "", "", nil
}

func newTestApp(t *testing.T) (*App, string) {
	t.Helper()
	root := t.TempDir()
	app := &App{
		Paths: runtime.Paths{
			ConfigDir:   filepath.Join(root, "etc"),
			DataDir:     filepath.Join(root, "var"),
			RuntimeDir:  filepath.Join(root, "runtime"),
			InstallDir:  filepath.Join(root, "install"),
			BinPath:     filepath.Join(root, "bin", "minimalist"),
			ServiceUnit: filepath.Join(root, "systemd", "minimalist.service"),
			SysctlPath:  filepath.Join(root, "sysctl", "99-minimalist-router.conf"),
		},
		Runner: fakeRunner{
			runFn: func(name string, args ...string) error {
				if name == "systemctl" && len(args) >= 2 && args[0] == "is-active" {
					return errors.New("inactive")
				}
				if name == "systemctl" && len(args) >= 2 && args[0] == "is-enabled" {
					return errors.New("disabled")
				}
				return nil
			},
			outputFn: func(name string, args ...string) (string, string, error) {
				if name == "journalctl" {
					return "", "", nil
				}
				return "", "", errors.New("unavailable")
			},
		},
		Client: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return nil, errors.New("unavailable")
			}),
		},
		Stdout: &bytes.Buffer{},
		Stderr: &bytes.Buffer{},
	}
	return app, root
}

func newTestAppWithEnabledManualNode(t *testing.T) *App {
	t.Helper()
	app, _ := newTestApp(t)
	app.Stdin = strings.NewReader("trojan://password@example.org:443?security=tls#service-node\n")
	if err := app.ImportLinks(); err != nil {
		t.Fatalf("import links: %v", err)
	}
	if err := app.SetNodeEnabled(1, true); err != nil {
		t.Fatalf("enable node: %v", err)
	}
	return app
}

func hasRecordedCall(calls []commandCall, name string, want ...string) bool {
	for _, call := range calls {
		if call.name != name {
			continue
		}
		matched := true
		for _, part := range want {
			found := false
			for _, arg := range call.args {
				if arg == part {
					found = true
					break
				}
			}
			if !found {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

func hasArgSequence(args []string, want ...string) bool {
	if len(want) == 0 || len(want) > len(args) {
		return false
	}
	for i := 0; i <= len(args)-len(want); i++ {
		matched := true
		for j := range want {
			if args[i+j] != want[j] {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

func assertOnlyCutoverPreflightCalls(t *testing.T, calls []commandCall) {
	t.Helper()
	for _, call := range calls {
		if call.name != "systemctl" || len(call.args) != 3 {
			t.Fatalf("expected only cutover preflight calls, got %#v", calls)
		}
		if call.args[1] != "--quiet" {
			t.Fatalf("expected only cutover preflight calls, got %#v", calls)
		}
		if call.args[0] != "is-active" && call.args[0] != "is-enabled" {
			t.Fatalf("expected only cutover preflight calls, got %#v", calls)
		}
		if call.args[2] != "mihomo.service" && call.args[2] != "minimalist.service" {
			t.Fatalf("expected only cutover preflight calls, got %#v", calls)
		}
	}
}

func TestImportLinksPersistsManualNode(t *testing.T) {
	app, _ := newTestApp(t)
	app.Stdin = strings.NewReader("trojan://password@example.org:443?security=tls#demo-node\n")
	if err := app.ImportLinks(); err != nil {
		t.Fatalf("import links: %v", err)
	}
	body, err := os.ReadFile(app.Paths.StatePath())
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	text := string(body)
	for _, needle := range []string{`"name": "demo-node"`, `"enabled": false`, `"kind": "manual"`} {
		if !strings.Contains(text, needle) {
			t.Fatalf("missing %q in state:\n%s", needle, text)
		}
	}
}

func TestListNodesAndRemoveNodePersistUpdatedState(t *testing.T) {
	app, _ := newTestApp(t)
	app.Stdin = strings.NewReader(strings.Join([]string{
		"trojan://password@example.org:443?security=tls#first-node",
		"trojan://password@two.example.org:443?security=tls#second-node",
	}, "\n") + "\n")
	if err := app.ImportLinks(); err != nil {
		t.Fatalf("import links: %v", err)
	}
	if err := app.ListNodes(); err != nil {
		t.Fatalf("list nodes: %v", err)
	}
	output := app.Stdout.(*bytes.Buffer).String()
	for _, needle := range []string{
		"1\tfirst-node\t0\tmanual\t",
		"2\tsecond-node\t0\tmanual\t",
	} {
		if !strings.Contains(output, needle) {
			t.Fatalf("missing %q in node list:\n%s", needle, output)
		}
	}
	if err := app.RemoveNode(1); err != nil {
		t.Fatalf("remove node: %v", err)
	}
	st, err := state.Load(app.Paths.StatePath())
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if len(st.Nodes) != 1 || st.Nodes[0].Name != "second-node" {
		t.Fatalf("expected only second-node after remove, got %+v", st.Nodes)
	}
}

func TestTestNodesReportsDelayForEnabledNodes(t *testing.T) {
	app, _ := newTestApp(t)
	app.Stdin = strings.NewReader("trojan://password@example.org:443?security=tls#delay-node\n")
	if err := app.ImportLinks(); err != nil {
		t.Fatalf("import links: %v", err)
	}
	if err := app.SetNodeEnabled(1, true); err != nil {
		t.Fatalf("enable node: %v", err)
	}
	app.Client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.Path != "/proxies/delay-node/delay" {
				t.Fatalf("unexpected delay path: %s", req.URL.String())
			}
			if req.URL.Query().Get("timeout") != "5000" || req.URL.Query().Get("url") == "" {
				t.Fatalf("unexpected delay query: %s", req.URL.RawQuery)
			}
			if req.Header.Get("Authorization") == "" {
				t.Fatalf("expected controller secret header")
			}
			return textResponse(http.StatusOK, `{"delay":42}`), nil
		}),
	}
	if err := app.TestNodes(); err != nil {
		t.Fatalf("test nodes: %v", err)
	}
	if output := app.Stdout.(*bytes.Buffer).String(); !strings.Contains(output, "delay-node\t42ms") {
		t.Fatalf("unexpected test nodes output:\n%s", output)
	}
}

func TestTestNodesPrintsMessageWhenNoEnabledNodes(t *testing.T) {
	app, _ := newTestApp(t)
	app.Stdin = strings.NewReader("trojan://password@example.org:443?security=tls#disabled-node\n")
	if err := app.ImportLinks(); err != nil {
		t.Fatalf("import links: %v", err)
	}
	if err := app.TestNodes(); err != nil {
		t.Fatalf("test nodes: %v", err)
	}
	if output := app.Stdout.(*bytes.Buffer).String(); !strings.Contains(output, "暂无启用节点") {
		t.Fatalf("unexpected no-enabled-node output:\n%s", output)
	}
}

func TestTestNodesReportsControllerErrorsPerNode(t *testing.T) {
	app, _ := newTestApp(t)
	app.Stdin = strings.NewReader("trojan://password@example.org:443?security=tls#error-node\n")
	if err := app.ImportLinks(); err != nil {
		t.Fatalf("import links: %v", err)
	}
	if err := app.SetNodeEnabled(1, true); err != nil {
		t.Fatalf("enable node: %v", err)
	}
	app.Client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return textResponse(http.StatusBadGateway, `{"error":"bad gateway"}`), nil
		}),
	}
	if err := app.TestNodes(); err != nil {
		t.Fatalf("test nodes: %v", err)
	}
	if output := app.Stdout.(*bytes.Buffer).String(); !strings.Contains(output, "error-node\tERROR\thttp 502") {
		t.Fatalf("unexpected controller error output:\n%s", output)
	}
}

func TestRemoveNodeRejectsReferencedManualNode(t *testing.T) {
	for _, tc := range []struct {
		name string
		acl  bool
		kind string
	}{
		{name: "rule", acl: false, kind: "domain"},
		{name: "acl", acl: true, kind: "src-cidr"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app, _ := newTestApp(t)
			app.Stdin = strings.NewReader("trojan://password@example.org:443?security=tls#referenced-node\n")
			if err := app.ImportLinks(); err != nil {
				t.Fatalf("import links: %v", err)
			}
			if err := app.AddRule(tc.acl, tc.kind, "example.com", "referenced-node"); err != nil {
				t.Fatalf("add rule: %v", err)
			}
			err := app.RemoveNode(1)
			if err == nil || !strings.Contains(err.Error(), "node is referenced by rule") {
				t.Fatalf("expected referenced-node guard, got %v", err)
			}
			st, err := state.Load(app.Paths.StatePath())
			if err != nil {
				t.Fatalf("load state: %v", err)
			}
			if len(st.Nodes) != 1 || st.Nodes[0].Name != "referenced-node" {
				t.Fatalf("expected referenced node to remain, got %+v", st.Nodes)
			}
		})
	}
}

func TestListRulesAndRemoveRuleSupportACLAndMainRules(t *testing.T) {
	app, _ := newTestApp(t)
	app.Stdin = strings.NewReader("trojan://password@example.org:443?security=tls#rule-node\n")
	if err := app.ImportLinks(); err != nil {
		t.Fatalf("import links: %v", err)
	}
	if err := app.AddRule(false, "domain-suffix", "example.com", "DIRECT"); err != nil {
		t.Fatalf("add rule: %v", err)
	}
	if err := app.AddRule(true, "src", "192.168.2.10/32", "DIRECT"); err != nil {
		t.Fatalf("add acl: %v", err)
	}
	if err := app.ListRules(false); err != nil {
		t.Fatalf("list rules: %v", err)
	}
	if err := app.ListRules(true); err != nil {
		t.Fatalf("list acl: %v", err)
	}
	output := app.Stdout.(*bytes.Buffer).String()
	for _, needle := range []string{
		"1\tDOMAIN-SUFFIX,example.com,DIRECT",
		"1\tSRC-IP-CIDR,192.168.2.10/32,DIRECT",
	} {
		if !strings.Contains(output, needle) {
			t.Fatalf("missing %q in rule output:\n%s", needle, output)
		}
	}
	if err := app.RemoveRule(false, 1); err != nil {
		t.Fatalf("remove rule: %v", err)
	}
	if err := app.RemoveRule(true, 1); err != nil {
		t.Fatalf("remove acl: %v", err)
	}
	st, err := state.Load(app.Paths.StatePath())
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if len(st.Rules) != 0 || len(st.ACL) != 0 {
		t.Fatalf("expected empty rules after removal, got rules=%+v acl=%+v", st.Rules, st.ACL)
	}
}

func TestListSubscriptionsAndMenuViewPrintCurrentState(t *testing.T) {
	app, _ := newTestApp(t)
	if err := app.AddSubscription("list-sub", "https://subscription.example.com/list.txt", true); err != nil {
		t.Fatalf("add subscription: %v", err)
	}
	st, err := state.Load(app.Paths.StatePath())
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	st.Subscriptions[0].Cache.LastSuccessAt = "2026-04-27T10:00:00+08:00"
	st.Subscriptions[0].Cache.LastError = "boom"
	st.Subscriptions[0].Enumeration.LastCount = 2
	if err := state.Save(app.Paths.StatePath(), st); err != nil {
		t.Fatalf("save state: %v", err)
	}
	if err := app.ListSubscriptions(); err != nil {
		t.Fatalf("list subscriptions: %v", err)
	}
	app.Stdout = &bytes.Buffer{}
	app.Stdin = strings.NewReader("1\n")
	if err := app.subscriptionsMenu(bufio.NewReader(app.Stdin)); err != nil {
		t.Fatalf("subscriptions menu: %v", err)
	}
	output := app.Stdout.(*bytes.Buffer).String()
	for _, needle := range []string{
		"1) 查看订阅",
		"6) 立即更新订阅",
		"1\tlist-sub\thttps://subscription.example.com/list.txt\t1\t2026-04-27T10:00:00+08:00\t2\tboom",
	} {
		if !strings.Contains(output, needle) {
			t.Fatalf("missing %q in subscriptions output:\n%s", needle, output)
		}
	}
}

func TestSubscriptionsMenuUpdateRefreshesEnabledSubscriptions(t *testing.T) {
	app, _ := newTestApp(t)
	if err := app.AddSubscription("menu-update", "https://subscription.example.com/menu.txt", true); err != nil {
		t.Fatalf("add subscription: %v", err)
	}
	app.Client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.String() != "https://subscription.example.com/menu.txt" {
				t.Fatalf("unexpected subscription fetch: %s", req.URL.String())
			}
			return textResponse(http.StatusOK, "trojan://password@example.org:443?security=tls#menu-sub-node\n"), nil
		}),
	}
	app.Stdin = strings.NewReader("6\n")
	if err := app.subscriptionsMenu(bufio.NewReader(app.Stdin)); err != nil {
		t.Fatalf("subscriptions menu update: %v", err)
	}
	st, err := state.Load(app.Paths.StatePath())
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if st.Subscriptions[0].Enumeration.LastCount != 1 || st.Subscriptions[0].Cache.LastSuccessAt == "" {
		t.Fatalf("expected subscription update state, got %+v", st.Subscriptions[0])
	}
	if len(st.Nodes) != 1 || st.Nodes[0].Name != "menu-sub-node" {
		t.Fatalf("expected subscription node from menu update, got %+v", st.Nodes)
	}
}

func TestRulesRepoCommandsExposeAndMutateRepoState(t *testing.T) {
	app, _ := newTestApp(t)
	if err := app.RulesRepoAdd("fcm-site", "codex.example.com"); err != nil {
		t.Fatalf("rules repo add: %v", err)
	}
	if err := app.RulesRepoSummary(); err != nil {
		t.Fatalf("rules repo summary: %v", err)
	}
	if err := app.RulesRepoEntries("fcm-site", "codex"); err != nil {
		t.Fatalf("rules repo entries: %v", err)
	}
	if err := app.RulesRepoFind("codex"); err != nil {
		t.Fatalf("rules repo find: %v", err)
	}
	output := app.Stdout.(*bytes.Buffer).String()
	for _, needle := range []string{
		"规则仓库:",
		"fcm-site",
		"codex.example.com",
		"matched=1",
	} {
		if !strings.Contains(output, needle) {
			t.Fatalf("missing %q in rules repo output:\n%s", needle, output)
		}
	}
	if err := app.RulesRepoRemove("fcm-site", "codex.example.com"); err != nil {
		t.Fatalf("rules repo remove: %v", err)
	}
	if err := app.RulesRepoAdd("fcm-site", "codex.example.com"); err != nil {
		t.Fatalf("rules repo re-add: %v", err)
	}
	lines, err := rulesrepo.ListEntries(app.Paths.RulesRepoPath(), "fcm-site", "codex")
	if err != nil {
		t.Fatalf("list entries after re-add: %v", err)
	}
	if len(lines) == 0 {
		t.Fatalf("expected codex entry after re-add")
	}
	indexText, _, ok := strings.Cut(lines[0], "\t")
	if !ok {
		t.Fatalf("unexpected entry line: %q", lines[0])
	}
	index, err := strconv.Atoi(indexText)
	if err != nil {
		t.Fatalf("parse entry index: %v", err)
	}
	if err := app.RulesRepoRemoveIndex("fcm-site", index); err != nil {
		t.Fatalf("rules repo remove index: %v", err)
	}
	lines, err = rulesrepo.ListEntries(app.Paths.RulesRepoPath(), "fcm-site", "codex")
	if err != nil {
		t.Fatalf("list entries after remove-index: %v", err)
	}
	if len(lines) != 0 {
		t.Fatalf("expected codex entry removed, got %#v", lines)
	}
}

func TestRulesRepoSummaryReturnsManifestError(t *testing.T) {
	app, _ := newTestApp(t)
	if err := rulesrepo.InitDefaultRepo(filepath.Dir(app.Paths.RulesRepoPath())); err != nil {
		t.Fatalf("init rules repo: %v", err)
	}
	if err := os.WriteFile(app.Paths.RulesRepoPath(), []byte("bad: [\n"), 0o640); err != nil {
		t.Fatalf("write invalid manifest: %v", err)
	}
	err := app.RulesRepoSummary()
	if err == nil || !strings.Contains(err.Error(), "parse manifest") {
		t.Fatalf("expected manifest parse error, got %v", err)
	}
}

func TestRulesRepoEntriesReturnsUnknownRulesetError(t *testing.T) {
	app, _ := newTestApp(t)
	err := app.RulesRepoEntries("missing-ruleset", "")
	if err == nil || !strings.Contains(err.Error(), "unknown ruleset: missing-ruleset") {
		t.Fatalf("expected unknown ruleset error, got %v", err)
	}
}

func TestRulesRepoFindRejectsEmptyKeyword(t *testing.T) {
	app, _ := newTestApp(t)
	err := app.RulesRepoFind("   ")
	if err == nil || !strings.Contains(err.Error(), "empty keyword") {
		t.Fatalf("expected empty keyword error, got %v", err)
	}
}

func TestRulesRepoAddRejectsInvalidEntry(t *testing.T) {
	app, _ := newTestApp(t)
	err := app.RulesRepoAdd("pt", "bad entry")
	if err == nil || !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("expected invalid entry error, got %v", err)
	}
}

func TestRulesRepoRemoveIndexRejectsOutOfRange(t *testing.T) {
	app, _ := newTestApp(t)
	err := app.RulesRepoRemoveIndex("pt", 9999)
	if err == nil || !strings.Contains(err.Error(), "entry index out of range") {
		t.Fatalf("expected index out of range error, got %v", err)
	}
}

func TestRulesRepoWrappersSurfaceEnsureAllErrors(t *testing.T) {
	app, _ := newTestApp(t)
	if err := os.WriteFile(app.Paths.ConfigDir, []byte("blocked"), 0o640); err != nil {
		t.Fatalf("write blocking config dir: %v", err)
	}
	for _, tc := range []struct {
		name string
		err  error
	}{
		{"add", app.RulesRepoAdd("pt", "codex.example.com")},
		{"remove", app.RulesRepoRemove("pt", "codex.example.com")},
		{"remove-index", app.RulesRepoRemoveIndex("pt", 1)},
		{"summary", app.RulesRepoSummary()},
		{"entries", app.RulesRepoEntries("pt", "")},
		{"find", app.RulesRepoFind("codex")},
	} {
		if tc.err == nil {
			t.Fatalf("expected ensureAll failure for %s", tc.name)
		}
	}
}

func TestImportLinksReportsUnsupportedOnlyAndMixedSkippedInputs(t *testing.T) {
	app, _ := newTestApp(t)
	app.Stdin = strings.NewReader("socks5://proxy.example.com:1080#legacy\n")
	err := app.ImportLinks()
	if err == nil || !strings.Contains(err.Error(), "没有读取到有效节点") {
		t.Fatalf("expected no valid node error, got %v", err)
	}
	if !strings.Contains(app.Stderr.(*bytes.Buffer).String(), "有 1 条链接因协议不受支持而被跳过") {
		t.Fatalf("unexpected stderr for unsupported input:\n%s", app.Stderr.(*bytes.Buffer).String())
	}

	app, _ = newTestApp(t)
	app.Stdin = strings.NewReader(strings.Join([]string{
		"socks5://proxy.example.com:1080#legacy",
		"trojan://password@example.org:443?security=tls#mixed-node",
	}, "\n") + "\n")
	if err := app.ImportLinks(); err != nil {
		t.Fatalf("import mixed links: %v", err)
	}
	output := app.Stdout.(*bytes.Buffer).String()
	for _, needle := range []string{
		"已处理 1 条节点",
		"有 1 条链接因协议不受支持而被跳过",
	} {
		if !strings.Contains(output, needle) {
			t.Fatalf("missing %q in import output:\n%s", needle, output)
		}
	}
}

func TestUpdateSubscriptionsRecordsHTTPAndTransportErrors(t *testing.T) {
	app, _ := newTestApp(t)
	if err := app.AddSubscription("http-fail", "https://subscription.example.com/http-fail.txt", true); err != nil {
		t.Fatalf("add subscription: %v", err)
	}
	app.Client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return textResponse(http.StatusBadGateway, "bad gateway"), nil
		}),
	}
	if err := app.UpdateSubscriptions(); err != nil {
		t.Fatalf("update subscriptions http fail: %v", err)
	}
	st, err := state.Load(app.Paths.StatePath())
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if st.Subscriptions[0].Cache.LastAttemptAt == "" || st.Subscriptions[0].Cache.LastError != "http 502" {
		t.Fatalf("expected http error recorded, got %+v", st.Subscriptions[0].Cache)
	}
	if st.Subscriptions[0].Cache.LastSuccessAt != "" || st.Subscriptions[0].Enumeration.LastCount != 0 {
		t.Fatalf("did not expect success fields after http failure, got %+v", st.Subscriptions[0])
	}
	if !strings.Contains(app.Stderr.(*bytes.Buffer).String(), "http-fail: http 502") {
		t.Fatalf("unexpected stderr after http failure:\n%s", app.Stderr.(*bytes.Buffer).String())
	}

	app, _ = newTestApp(t)
	if err := app.AddSubscription("transport-fail", "https://subscription.example.com/transport-fail.txt", true); err != nil {
		t.Fatalf("add subscription: %v", err)
	}
	app.Client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return nil, errors.New("dial tcp timeout")
		}),
	}
	if err := app.UpdateSubscriptions(); err != nil {
		t.Fatalf("update subscriptions transport fail: %v", err)
	}
	st, err = state.Load(app.Paths.StatePath())
	if err != nil {
		t.Fatalf("reload state: %v", err)
	}
	if !strings.Contains(st.Subscriptions[0].Cache.LastError, "dial tcp timeout") {
		t.Fatalf("expected transport error recorded, got %+v", st.Subscriptions[0].Cache)
	}
	if !strings.Contains(app.Stderr.(*bytes.Buffer).String(), "transport-fail: Get") || !strings.Contains(app.Stderr.(*bytes.Buffer).String(), "dial tcp timeout") {
		t.Fatalf("unexpected stderr after transport failure:\n%s", app.Stderr.(*bytes.Buffer).String())
	}
}

func TestUpdateSubscriptionsFailurePreservesPreviousSuccessState(t *testing.T) {
	app, _ := newTestApp(t)
	if err := app.AddSubscription("flaky-sub", "https://subscription.example.com/flaky.txt", true); err != nil {
		t.Fatalf("add subscription: %v", err)
	}
	app.Client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return textResponse(http.StatusOK, "trojan://password@example.org:443?security=tls#stable-node\n"), nil
		}),
	}
	if err := app.UpdateSubscriptions(); err != nil {
		t.Fatalf("first update subscriptions: %v", err)
	}
	st, err := state.Load(app.Paths.StatePath())
	if err != nil {
		t.Fatalf("load state after success: %v", err)
	}
	successAt := st.Subscriptions[0].Cache.LastSuccessAt
	lastCount := st.Subscriptions[0].Enumeration.LastCount
	if successAt == "" || lastCount != 1 || len(st.Nodes) != 1 {
		t.Fatalf("expected successful subscription state, got %+v", st)
	}

	app.Client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return textResponse(http.StatusServiceUnavailable, "temporary"), nil
		}),
	}
	if err := app.UpdateSubscriptions(); err != nil {
		t.Fatalf("second update subscriptions: %v", err)
	}
	st, err = state.Load(app.Paths.StatePath())
	if err != nil {
		t.Fatalf("load state after failure: %v", err)
	}
	sub := st.Subscriptions[0]
	if sub.Cache.LastSuccessAt != successAt || sub.Enumeration.LastCount != lastCount {
		t.Fatalf("expected previous success fields to remain, got %+v", sub)
	}
	if sub.Cache.LastError != "http 503" {
		t.Fatalf("expected latest failure to be recorded, got %+v", sub.Cache)
	}
	if len(st.Nodes) != 1 || st.Nodes[0].Name != "stable-node" {
		t.Fatalf("expected previous subscription node to remain, got %+v", st.Nodes)
	}
}

func TestUpdateSubscriptionsSkipsDisabledSubscriptions(t *testing.T) {
	app, _ := newTestApp(t)
	if err := app.AddSubscription("disabled-sub", "https://subscription.example.com/disabled.txt", true); err != nil {
		t.Fatalf("add subscription: %v", err)
	}
	if err := app.SetSubscriptionEnabled(1, false); err != nil {
		t.Fatalf("disable subscription: %v", err)
	}
	app.Client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			t.Fatalf("disabled subscription should not be fetched: %s", req.URL.String())
			return nil, nil
		}),
	}
	if err := app.UpdateSubscriptions(); err != nil {
		t.Fatalf("update subscriptions: %v", err)
	}
	st, err := state.Load(app.Paths.StatePath())
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if st.Subscriptions[0].Cache.LastAttemptAt != "" || st.Subscriptions[0].Cache.LastError != "" {
		t.Fatalf("disabled subscription should not update cache state, got %+v", st.Subscriptions[0].Cache)
	}
	if _, err := os.Stat(app.Paths.SubscriptionFile(st.Subscriptions[0].ID)); !os.IsNotExist(err) {
		t.Fatalf("disabled subscription should not create cache file, stat err=%v", err)
	}
}

func TestUpdateSubscriptionsProcessesOnlyEnabledSubscriptions(t *testing.T) {
	app, _ := newTestApp(t)
	if err := app.AddSubscription("enabled-sub", "https://subscription.example.com/enabled.txt", true); err != nil {
		t.Fatalf("add enabled subscription: %v", err)
	}
	if err := app.AddSubscription("disabled-sub", "https://subscription.example.com/disabled.txt", true); err != nil {
		t.Fatalf("add disabled subscription: %v", err)
	}
	if err := app.SetSubscriptionEnabled(2, false); err != nil {
		t.Fatalf("disable second subscription: %v", err)
	}
	app.Client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.String() != "https://subscription.example.com/enabled.txt" {
				t.Fatalf("unexpected subscription fetch: %s", req.URL.String())
			}
			return textResponse(http.StatusOK, "trojan://password@example.org:443?security=tls#enabled-node\n"), nil
		}),
	}
	if err := app.UpdateSubscriptions(); err != nil {
		t.Fatalf("update subscriptions: %v", err)
	}
	st, err := state.Load(app.Paths.StatePath())
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if st.Subscriptions[0].Cache.LastAttemptAt == "" || st.Subscriptions[0].Cache.LastSuccessAt == "" {
		t.Fatalf("expected enabled subscription to update cache, got %+v", st.Subscriptions[0].Cache)
	}
	if st.Subscriptions[1].Cache.LastAttemptAt != "" || st.Subscriptions[1].Cache.LastSuccessAt != "" {
		t.Fatalf("expected disabled subscription to remain untouched, got %+v", st.Subscriptions[1].Cache)
	}
	if len(st.Nodes) != 1 || st.Nodes[0].Source.Kind != "subscription" || st.Nodes[0].Source.ID != st.Subscriptions[0].ID {
		t.Fatalf("expected only enabled subscription node to be imported, got %+v", st.Nodes)
	}
}

func TestUpdateSubscriptionsFailsWhenSubscriptionDirCannotBeCreated(t *testing.T) {
	app, _ := newTestApp(t)
	if err := app.AddSubscription("dir-blocked", "https://subscription.example.com/dir-blocked.txt", true); err != nil {
		t.Fatalf("add subscription: %v", err)
	}
	blockRoot := filepath.Join(t.TempDir(), "blocked-runtime")
	app.Paths.RuntimeDir = blockRoot
	app.Client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return textResponse(http.StatusOK, "trojan://password@example.org:443?security=tls#blocked\n"), nil
		}),
	}
	if err := os.MkdirAll(filepath.Dir(app.Paths.ProviderDir()), 0o755); err != nil {
		t.Fatalf("mkdir runtime parent: %v", err)
	}
	if err := os.WriteFile(app.Paths.ProviderDir(), []byte("occupied"), 0o640); err != nil {
		t.Fatalf("write blocking file: %v", err)
	}
	err := app.UpdateSubscriptions()
	if err == nil || (!strings.Contains(err.Error(), "not a directory") && !strings.Contains(err.Error(), "file exists")) {
		t.Fatalf("expected subscription dir creation error, got %v", err)
	}
}

func TestRulesAndACLMenuAddsRuleAndPromptStringKeepsDefaultOnBlankInput(t *testing.T) {
	app, _ := newTestApp(t)
	app.Stdin = strings.NewReader("trojan://password@example.org:443?security=tls#menu-rule-node\n")
	if err := app.ImportLinks(); err != nil {
		t.Fatalf("import links: %v", err)
	}
	if err := app.SetNodeEnabled(1, true); err != nil {
		t.Fatalf("enable node: %v", err)
	}

	reader := bufio.NewReader(strings.NewReader("2\ndomain\nmenu.example.com\nAUTO\n"))
	if err := app.rulesAndACLMenu(reader); err != nil {
		t.Fatalf("rules and acl menu add: %v", err)
	}
	st, err := state.Load(app.Paths.StatePath())
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if len(st.Rules) != 1 || st.Rules[0].Target != "AUTO" || st.Rules[0].Pattern != "menu.example.com" {
		t.Fatalf("unexpected rule after menu add: %+v", st.Rules)
	}

	var out bytes.Buffer
	if got := promptString(bufio.NewReader(strings.NewReader("\n")), &out, "index", "7"); got != "7" {
		t.Fatalf("expected promptString to keep default, got %q", got)
	}
	if !strings.Contains(out.String(), "index [7]: ") {
		t.Fatalf("unexpected prompt output: %q", out.String())
	}
}

func TestRulesAndACLMenuRemovesRuleByIndex(t *testing.T) {
	app, _ := newTestApp(t)
	if err := app.AddRule(false, "domain", "menu.example.com", "DIRECT"); err != nil {
		t.Fatalf("add rule: %v", err)
	}
	reader := bufio.NewReader(strings.NewReader("3\n1\n"))
	if err := app.rulesAndACLMenu(reader); err != nil {
		t.Fatalf("rules and acl menu remove: %v", err)
	}
	st, err := state.Load(app.Paths.StatePath())
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if len(st.Rules) != 0 {
		t.Fatalf("expected rules to be removed, got %+v", st.Rules)
	}
}

func TestRulesAndACLMenuSupportsACLAddAndRemove(t *testing.T) {
	app, _ := newTestApp(t)
	reader := bufio.NewReader(strings.NewReader("5\nsrc\n192.168.2.10/32\nDIRECT\n"))
	if err := app.rulesAndACLMenu(reader); err != nil {
		t.Fatalf("rules and acl menu acl add: %v", err)
	}
	st, err := state.Load(app.Paths.StatePath())
	if err != nil {
		t.Fatalf("load state after acl add: %v", err)
	}
	if len(st.ACL) != 1 || st.ACL[0].Kind != "src-cidr" || st.ACL[0].Pattern != "192.168.2.10/32" {
		t.Fatalf("unexpected acl after add: %+v", st.ACL)
	}
	reader = bufio.NewReader(strings.NewReader("6\n1\n"))
	if err := app.rulesAndACLMenu(reader); err != nil {
		t.Fatalf("rules and acl menu acl remove: %v", err)
	}
	st, err = state.Load(app.Paths.StatePath())
	if err != nil {
		t.Fatalf("load state after acl remove: %v", err)
	}
	if len(st.ACL) != 0 {
		t.Fatalf("expected acl to be removed, got %+v", st.ACL)
	}
}

func TestMenuDispatchesMainActionsAndIgnoresInvalidChoice(t *testing.T) {
	app, _ := newTestApp(t)
	oldGeteuid := geteuid
	geteuid = func() int { return 0 }
	defer func() { geteuid = oldGeteuid }()

	app.Stdin = strings.NewReader(strings.Join([]string{
		"x",
		"1",
		"2",
		"1",
		"4",
		"1",
		"6",
		"1",
		"6",
		"4",
		"8",
		"1",
		"0",
	}, "\n") + "\n")
	if err := app.Menu(); err != nil {
		t.Fatalf("menu: %v", err)
	}
	output := app.Stdout.(*bytes.Buffer).String()
	for _, needle := range []string{
		"无效选择",
		"部署完成，请先 import-links 或 subscriptions update 后再启动服务",
		"1) 查看订阅",
		"1) 查看自定义规则",
		"mixed-port=7890",
	} {
		if !strings.Contains(output, needle) {
			t.Fatalf("missing %q in menu output:\n%s", needle, output)
		}
	}
}

func TestDeployMenuIgnoresInvalidChoiceThenRendersConfig(t *testing.T) {
	app := newTestAppWithEnabledManualNode(t)
	reader := bufio.NewReader(strings.NewReader("x\n2\n"))
	if err := app.deployMenu(reader); err != nil {
		t.Fatalf("deploy menu: %v", err)
	}
	body, err := os.ReadFile(app.Paths.RuntimeConfig())
	if err != nil {
		t.Fatalf("read runtime config: %v", err)
	}
	if !strings.Contains(string(body), "proxy-groups:") {
		t.Fatalf("unexpected runtime config:\n%s", string(body))
	}
	output := app.Stdout.(*bytes.Buffer).String()
	if !strings.Contains(output, "无效选择") || !strings.Contains(output, "2) 重新渲染配置") {
		t.Fatalf("unexpected deploy menu output:\n%s", output)
	}
}

func TestNodesMenuReturnsMutationErrors(t *testing.T) {
	app, _ := newTestApp(t)
	app.Stdin = strings.NewReader("trojan://password@example.org:443?security=tls#menu-node\n")
	if err := app.ImportLinks(); err != nil {
		t.Fatalf("import links: %v", err)
	}
	reader := bufio.NewReader(strings.NewReader("4\n1\n\n"))
	err := app.nodesMenu(reader)
	if err == nil || !strings.Contains(err.Error(), "node name is empty") {
		t.Fatalf("expected rename validation error, got %v", err)
	}
	if !strings.Contains(app.Stdout.(*bytes.Buffer).String(), "4) 节点改名") {
		t.Fatalf("expected nodes menu output, got:\n%s", app.Stdout.(*bytes.Buffer).String())
	}
}

func TestSetNodeEnabledUpdatesManualNodesAndRejectsSubscriptionNodes(t *testing.T) {
	app, _ := newTestApp(t)
	app.Stdin = strings.NewReader("trojan://password@example.org:443?security=tls#manual-node\n")
	if err := app.ImportLinks(); err != nil {
		t.Fatalf("import links: %v", err)
	}
	if err := app.SetNodeEnabled(1, true); err != nil {
		t.Fatalf("enable manual node: %v", err)
	}
	if err := app.SetNodeEnabled(1, false); err != nil {
		t.Fatalf("disable manual node: %v", err)
	}
	st, err := state.Load(app.Paths.StatePath())
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if st.Nodes[0].Enabled {
		t.Fatalf("expected manual node to be disabled, got %+v", st.Nodes[0])
	}

	app, _ = newTestApp(t)
	app.Client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return textResponse(http.StatusOK, "trojan://password@example.org:443?security=tls#sub-node\n"), nil
		}),
	}
	if err := app.AddSubscription("sub-node", "https://subscription.example.com/sub.txt", true); err != nil {
		t.Fatalf("add subscription: %v", err)
	}
	if err := app.UpdateSubscriptions(); err != nil {
		t.Fatalf("update subscriptions: %v", err)
	}
	err = app.SetNodeEnabled(1, true)
	if err == nil || !strings.Contains(err.Error(), "subscription node is provider-managed") {
		t.Fatalf("expected subscription ownership error, got %v", err)
	}
}

func TestNodeMutationCommandsRejectOutOfRangeIndexes(t *testing.T) {
	app, _ := newTestApp(t)
	if err := app.RenameNode(1, "missing"); err == nil || !strings.Contains(err.Error(), "node index out of range") {
		t.Fatalf("expected rename range error, got %v", err)
	}
	if err := app.SetNodeEnabled(1, true); err == nil || !strings.Contains(err.Error(), "node index out of range") {
		t.Fatalf("expected enable range error, got %v", err)
	}
}

func TestRenameNodeRejectsEmptyNamesAndSubscriptionNodes(t *testing.T) {
	app, _ := newTestApp(t)
	if err := app.RenameNode(1, "   "); err == nil || !strings.Contains(err.Error(), "node name is empty") {
		t.Fatalf("expected empty rename error, got %v", err)
	}

	app, _ = newTestApp(t)
	app.Client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return textResponse(http.StatusOK, "trojan://password@example.org:443?security=tls#sub-rename\n"), nil
		}),
	}
	if err := app.AddSubscription("rename-sub", "https://subscription.example.com/rename.txt", true); err != nil {
		t.Fatalf("add subscription: %v", err)
	}
	if err := app.UpdateSubscriptions(); err != nil {
		t.Fatalf("update subscriptions: %v", err)
	}
	if err := app.RenameNode(1, "manual-name"); err == nil || !strings.Contains(err.Error(), "subscription node is provider-managed") {
		t.Fatalf("expected subscription rename guard, got %v", err)
	}
}

func TestRenameNodeUpdatesRuleAndACLTargets(t *testing.T) {
	app, _ := newTestApp(t)
	app.Stdin = strings.NewReader("trojan://password@example.org:443?security=tls#old-name\n")
	if err := app.ImportLinks(); err != nil {
		t.Fatalf("import links: %v", err)
	}
	if err := app.AddRule(false, "domain", "rename.example.com", "old-name"); err != nil {
		t.Fatalf("add rule: %v", err)
	}
	if err := app.AddRule(true, "src", "192.168.2.9/32", "old-name"); err != nil {
		t.Fatalf("add acl: %v", err)
	}
	if err := app.RenameNode(1, "new-name"); err != nil {
		t.Fatalf("rename node: %v", err)
	}
	st, err := state.Load(app.Paths.StatePath())
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if st.Nodes[0].Name != "new-name" {
		t.Fatalf("expected node name to be updated, got %+v", st.Nodes[0])
	}
	if st.Rules[0].Target != "new-name" || st.ACL[0].Target != "new-name" {
		t.Fatalf("expected rule targets to follow rename, got rules=%+v acl=%+v", st.Rules, st.ACL)
	}
}

func TestSetSubscriptionEnabledKeepsNodesWhenEnablingAndRejectsOutOfRange(t *testing.T) {
	app, _ := newTestApp(t)
	app.Client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return textResponse(http.StatusOK, "trojan://password@example.org:443?security=tls#sub-keep\n"), nil
		}),
	}
	if err := app.AddSubscription("keep-sub", "https://subscription.example.com/keep.txt", true); err != nil {
		t.Fatalf("add subscription: %v", err)
	}
	if err := app.UpdateSubscriptions(); err != nil {
		t.Fatalf("update subscriptions: %v", err)
	}
	if err := app.SetSubscriptionEnabled(1, true); err != nil {
		t.Fatalf("enable subscription: %v", err)
	}
	st, err := state.Load(app.Paths.StatePath())
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if !st.Subscriptions[0].Enabled || len(st.Nodes) != 1 || st.Nodes[0].Source.Kind != "subscription" {
		t.Fatalf("expected subscription nodes to stay present, got %+v", st)
	}
	if err := app.SetSubscriptionEnabled(2, true); err == nil || !strings.Contains(err.Error(), "subscription index out of range") {
		t.Fatalf("expected subscription range error, got %v", err)
	}
}

func TestAddSubscriptionRejectsEmptyInputsAndUpdatesExistingURL(t *testing.T) {
	app, _ := newTestApp(t)
	if err := app.AddSubscription("", "https://subscription.example.com/sub.txt", true); err == nil || !strings.Contains(err.Error(), "subscription name is empty") {
		t.Fatalf("expected empty name error, got %v", err)
	}
	if err := app.AddSubscription("demo", "   ", true); err == nil || !strings.Contains(err.Error(), "subscription url is empty") {
		t.Fatalf("expected empty url error, got %v", err)
	}
	if err := app.AddSubscription("first", "https://subscription.example.com/sub.txt", true); err != nil {
		t.Fatalf("add subscription: %v", err)
	}
	if err := app.AddSubscription("second", "https://subscription.example.com/sub.txt", false); err != nil {
		t.Fatalf("update existing subscription: %v", err)
	}
	st, err := state.Load(app.Paths.StatePath())
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if len(st.Subscriptions) != 1 {
		t.Fatalf("expected one subscription after dedupe, got %+v", st.Subscriptions)
	}
	if st.Subscriptions[0].Name != "second" || st.Subscriptions[0].Enabled {
		t.Fatalf("expected existing subscription to be updated, got %+v", st.Subscriptions[0])
	}
}

func TestContainerBypassIPsAndEnsureChainHandleRunnerResponses(t *testing.T) {
	app, _ := newTestApp(t)
	var calls []commandCall
	app.Runner = fakeRunner{
		runFn: func(name string, args ...string) error {
			calls = append(calls, commandCall{name: name, args: append([]string{}, args...)})
			if name == "iptables" && len(args) >= 6 && args[4] == "-S" && args[5] == "CHAIN_EXISTS" {
				return nil
			}
			if name == "iptables" && len(args) >= 6 && args[4] == "-S" && args[5] == "CHAIN_NEW" {
				return errors.New("missing")
			}
			return nil
		},
		outputFn: func(name string, args ...string) (string, string, error) {
			if name == "docker" && len(args) >= 2 && args[1] == "alpha" {
				return "172.18.0.2\n172.18.0.3\n", "", nil
			}
			if name == "docker" && len(args) >= 2 && args[1] == "beta" {
				return "172.18.0.2\n", "", nil
			}
			return "", "", errors.New("missing")
		},
	}
	ips := app.containerBypassIPs([]string{"alpha", "beta", "missing"})
	if strings.Join(ips, ",") != "172.18.0.2/32,172.18.0.3/32" {
		t.Fatalf("unexpected container bypass ips: %#v", ips)
	}
	if err := app.ensureChain("mangle", "CHAIN_EXISTS"); err != nil {
		t.Fatalf("ensure existing chain: %v", err)
	}
	if err := app.ensureChain("mangle", "CHAIN_NEW"); err != nil {
		t.Fatalf("ensure new chain: %v", err)
	}
	if !hasRecordedCall(calls, "iptables", "-F", "CHAIN_EXISTS") {
		t.Fatalf("expected flush call for existing chain, calls=%#v", calls)
	}
	if !hasRecordedCall(calls, "iptables", "-N", "CHAIN_NEW") {
		t.Fatalf("expected create call for missing chain, calls=%#v", calls)
	}
}

func TestDeleteIPRuleRetriesUntilRuleIsMissing(t *testing.T) {
	app, _ := newTestApp(t)
	var calls []commandCall
	hits := 0
	app.Runner = fakeRunner{
		runFn: func(name string, args ...string) error {
			calls = append(calls, commandCall{name: name, args: append([]string{}, args...)})
			if name == "ip" && len(args) >= 9 && args[0] == "-4" && args[1] == "rule" && args[2] == "del" {
				hits++
				return nil
			}
			return nil
		},
		outputFn: func(name string, args ...string) (string, string, error) {
			if name == "ip" && len(args) >= 3 && args[0] == "-4" && args[1] == "rule" && args[2] == "show" {
				if hits == 0 {
					return "100: from all fwmark 0x2333 lookup 233\n", "", nil
				}
				return "", "", nil
			}
			return "", "", nil
		},
	}
	if err := app.deleteIPRule("233", "100"); err != nil {
		t.Fatalf("delete ip rule: %v", err)
	}
	if hits != 1 {
		t.Fatalf("expected delete loop to stop after missing rule, hits=%d calls=%#v", hits, calls)
	}
	if !hasRecordedCall(calls, "ip", "-4", "rule", "del", "fwmark", "9011", "table", "233", "priority", "100") {
		t.Fatalf("expected ip rule delete call, calls=%#v", calls)
	}
}

func TestDeleteIPRuleReturnsShowError(t *testing.T) {
	app, _ := newTestApp(t)
	app.Runner = fakeRunner{
		outputFn: func(name string, args ...string) (string, string, error) {
			return "", "", errors.New("show failed")
		},
	}
	if err := app.deleteIPRule("233", "100"); err == nil || !strings.Contains(err.Error(), "show failed") {
		t.Fatalf("expected show error, got %v", err)
	}
}

func TestDeleteIPRuleReturnsDeleteErrorWhenRuleIsPresent(t *testing.T) {
	app, _ := newTestApp(t)
	app.Runner = fakeRunner{
		runFn: func(name string, args ...string) error {
			if name == "ip" && len(args) >= 9 && args[0] == "-4" && args[1] == "rule" && args[2] == "del" {
				return errors.New("delete failed")
			}
			return nil
		},
		outputFn: func(name string, args ...string) (string, string, error) {
			return "100: from all fwmark 0x2333 lookup 233\n", "", nil
		},
	}
	if err := app.deleteIPRule("233", "100"); err == nil || !strings.Contains(err.Error(), "delete failed") {
		t.Fatalf("expected delete error, got %v", err)
	}
}

func TestSetupWithoutProvidersDoesNotEnableService(t *testing.T) {
	app, _ := newTestApp(t)
	var calls []commandCall
	app.Runner = fakeRunner{
		runFn: func(name string, args ...string) error {
			calls = append(calls, commandCall{name: name, args: append([]string{}, args...)})
			if name == "systemctl" && len(args) >= 2 && args[0] == "is-active" {
				return errors.New("inactive")
			}
			if name == "systemctl" && len(args) >= 2 && args[0] == "is-enabled" {
				return errors.New("disabled")
			}
			if name == "iptables" {
				for _, arg := range args {
					if arg == "-C" || arg == "-S" {
						return errors.New("missing")
					}
				}
			}
			if name == "ip" && len(args) >= 4 && args[0] == "-4" && args[1] == "rule" && args[2] == "del" {
				return errors.New("missing")
			}
			return nil
		},
		outputFn: func(name string, args ...string) (string, string, error) {
			return "", "", nil
		},
	}
	if err := app.Setup(); err != nil {
		t.Fatalf("setup without providers: %v", err)
	}
	if hasRecordedCall(calls, "systemctl", "enable", "--now", "minimalist.service") {
		t.Fatalf("service should not be enabled without providers")
	}
	if !hasRecordedCall(calls, "systemctl", "daemon-reload") {
		t.Fatalf("expected daemon-reload call")
	}
	serviceBody, err := os.ReadFile(app.Paths.ServiceUnit)
	if err != nil {
		t.Fatalf("read service unit: %v", err)
	}
	if !strings.Contains(string(serviceBody), "ExecStartPre=+") || !strings.Contains(string(serviceBody), "minimalist apply-rules") {
		t.Fatalf("unexpected service unit:\n%s", string(serviceBody))
	}
	sysctlBody, err := os.ReadFile(app.Paths.SysctlPath)
	if err != nil {
		t.Fatalf("read sysctl: %v", err)
	}
	if !strings.Contains(string(sysctlBody), "net.ipv4.ip_forward = 1") {
		t.Fatalf("unexpected sysctl content:\n%s", string(sysctlBody))
	}
	output := app.Stdout.(*bytes.Buffer).String()
	if !strings.Contains(output, "部署完成，请先 import-links 或 subscriptions update 后再启动服务") {
		t.Fatalf("unexpected setup output:\n%s", output)
	}
}

func TestSetupWithProvidersEnablesService(t *testing.T) {
	app, _ := newTestApp(t)
	var calls []commandCall
	app.Runner = fakeRunner{
		runFn: func(name string, args ...string) error {
			calls = append(calls, commandCall{name: name, args: append([]string{}, args...)})
			if name == "iptables" {
				for _, arg := range args {
					if arg == "-C" || arg == "-S" {
						return errors.New("missing")
					}
				}
			}
			if name == "ip" && len(args) >= 4 && args[0] == "-4" && args[1] == "rule" && args[2] == "del" {
				return errors.New("missing")
			}
			return nil
		},
		outputFn: func(name string, args ...string) (string, string, error) {
			return "", "", nil
		},
	}
	app.Stdin = strings.NewReader("trojan://password@example.org:443?security=tls#setup-node\n")
	if err := app.ImportLinks(); err != nil {
		t.Fatalf("import links: %v", err)
	}
	if err := app.SetNodeEnabled(1, true); err != nil {
		t.Fatalf("enable node: %v", err)
	}
	if err := app.Setup(); err != nil {
		t.Fatalf("setup with providers: %v", err)
	}
	if !hasRecordedCall(calls, "systemctl", "enable", "--now", "minimalist.service") {
		t.Fatalf("expected setup to enable service, calls=%#v", calls)
	}
	output := app.Stdout.(*bytes.Buffer).String()
	if !strings.Contains(output, "部署完成，服务已启用") {
		t.Fatalf("unexpected setup output:\n%s", output)
	}
}

func TestSetupEnablesServiceWhenSubscriptionCacheIsReady(t *testing.T) {
	app, _ := newTestApp(t)
	var calls []commandCall
	app.Runner = fakeRunner{
		runFn: func(name string, args ...string) error {
			calls = append(calls, commandCall{name: name, args: append([]string{}, args...)})
			return nil
		},
		outputFn: func(name string, args ...string) (string, string, error) {
			return "", "", nil
		},
	}
	if err := app.AddSubscription("setup-sub", "https://subscription.example.com/setup.txt", true); err != nil {
		t.Fatalf("add subscription: %v", err)
	}
	st, err := state.Load(app.Paths.StatePath())
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if err := os.MkdirAll(app.Paths.SubscriptionDir(), 0o755); err != nil {
		t.Fatalf("mkdir subscription dir: %v", err)
	}
	if err := os.WriteFile(app.Paths.SubscriptionFile(st.Subscriptions[0].ID), []byte("trojan://password@example.org:443?security=tls#cached\n"), 0o640); err != nil {
		t.Fatalf("write subscription cache: %v", err)
	}
	if err := app.Setup(); err != nil {
		t.Fatalf("setup with subscription cache: %v", err)
	}
	if !hasRecordedCall(calls, "systemctl", "enable", "--now", "minimalist.service") {
		t.Fatalf("expected setup to enable service from subscription cache, calls=%#v", calls)
	}
}

func TestSetupDoesNotEnableServiceWhenSubscriptionCacheIsEmpty(t *testing.T) {
	app, _ := newTestApp(t)
	var calls []commandCall
	app.Runner = fakeRunner{
		runFn: func(name string, args ...string) error {
			calls = append(calls, commandCall{name: name, args: append([]string{}, args...)})
			return nil
		},
		outputFn: func(name string, args ...string) (string, string, error) {
			return "", "", nil
		},
	}
	if err := app.AddSubscription("empty-cache-sub", "https://subscription.example.com/empty.txt", true); err != nil {
		t.Fatalf("add subscription: %v", err)
	}
	st, err := state.Load(app.Paths.StatePath())
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if err := os.MkdirAll(app.Paths.SubscriptionDir(), 0o755); err != nil {
		t.Fatalf("mkdir subscription dir: %v", err)
	}
	if err := os.WriteFile(app.Paths.SubscriptionFile(st.Subscriptions[0].ID), nil, 0o640); err != nil {
		t.Fatalf("write empty subscription cache: %v", err)
	}
	if err := app.Setup(); err != nil {
		t.Fatalf("setup with empty subscription cache: %v", err)
	}
	if hasRecordedCall(calls, "systemctl", "enable", "--now", "minimalist.service") {
		t.Fatalf("service should not be enabled with an empty subscription cache, calls=%#v", calls)
	}
	if !strings.Contains(app.Stdout.(*bytes.Buffer).String(), "部署完成，请先 import-links 或 subscriptions update 后再启动服务") {
		t.Fatalf("unexpected setup output:\n%s", app.Stdout.(*bytes.Buffer).String())
	}
}

func TestSetupPropagatesEnableFailureWhenProvidersReady(t *testing.T) {
	app, _ := newTestApp(t)
	app.Runner = fakeRunner{
		runFn: func(name string, args ...string) error {
			if name == "systemctl" && len(args) >= 2 && args[0] == "enable" {
				return errors.New("enable failed")
			}
			if name == "iptables" {
				for _, arg := range args {
					if arg == "-C" || arg == "-S" {
						return errors.New("missing")
					}
				}
			}
			if name == "ip" && len(args) >= 4 && args[0] == "-4" && args[1] == "rule" && args[2] == "del" {
				return errors.New("missing")
			}
			return nil
		},
		outputFn: func(name string, args ...string) (string, string, error) {
			return "", "", nil
		},
	}
	app.Stdin = strings.NewReader("trojan://password@example.org:443?security=tls#setup-node\n")
	if err := app.ImportLinks(); err != nil {
		t.Fatalf("import links: %v", err)
	}
	if err := app.SetNodeEnabled(1, true); err != nil {
		t.Fatalf("enable node: %v", err)
	}
	err := app.Setup()
	if err == nil || !strings.Contains(err.Error(), "enable failed") {
		t.Fatalf("expected enable failure, got %v", err)
	}
	if strings.Contains(app.Stdout.(*bytes.Buffer).String(), "部署完成，服务已启用") {
		t.Fatalf("did not expect success output on enable failure:\n%s", app.Stdout.(*bytes.Buffer).String())
	}
}

func TestSetupPropagatesSysctlAndDaemonReloadFailures(t *testing.T) {
	app, _ := newTestApp(t)
	app.Runner = fakeRunner{
		runFn: func(name string, args ...string) error {
			if name == "sysctl" {
				return errors.New("sysctl failed")
			}
			return nil
		},
		outputFn: func(name string, args ...string) (string, string, error) {
			return "", "", nil
		},
	}
	if err := app.Setup(); err == nil || !strings.Contains(err.Error(), "sysctl failed") {
		t.Fatalf("expected sysctl failure, got %v", err)
	}

	app, _ = newTestApp(t)
	app.Runner = fakeRunner{
		runFn: func(name string, args ...string) error {
			if name == "systemctl" && len(args) >= 1 && args[0] == "daemon-reload" {
				return errors.New("daemon reload failed")
			}
			return nil
		},
		outputFn: func(name string, args ...string) (string, string, error) {
			return "", "", nil
		},
	}
	if err := app.Setup(); err == nil || !strings.Contains(err.Error(), "daemon reload failed") {
		t.Fatalf("expected daemon-reload failure, got %v", err)
	}
}

func TestSetupPropagatesRenderFilesRulesRepoError(t *testing.T) {
	app, _ := newTestApp(t)
	oldGeteuid := geteuid
	geteuid = func() int { return 0 }
	defer func() { geteuid = oldGeteuid }()

	if err := os.MkdirAll(filepath.Dir(app.Paths.RulesRepoPath()), 0o755); err != nil {
		t.Fatalf("mkdir rules repo dir: %v", err)
	}
	if err := os.WriteFile(app.Paths.RulesRepoPath(), []byte("rulesets: [\n"), 0o640); err != nil {
		t.Fatalf("write invalid manifest: %v", err)
	}
	var calls []commandCall
	app.Runner = fakeRunner{
		runFn: func(name string, args ...string) error {
			calls = append(calls, commandCall{name: name, args: append([]string{}, args...)})
			return nil
		},
	}
	if err := app.Setup(); err == nil || !strings.Contains(err.Error(), "parse manifest") {
		t.Fatalf("expected render files failure, got %v", err)
	}
	assertOnlyCutoverPreflightCalls(t, calls)
}

func TestSetupSurfaceEnsureAllFailureWhenRuntimeLayoutBlocked(t *testing.T) {
	app, _ := newTestApp(t)
	if err := os.WriteFile(app.Paths.RuntimeDir, []byte("blocked"), 0o640); err != nil {
		t.Fatalf("write blocking runtime dir: %v", err)
	}
	if err := app.Setup(); err == nil {
		t.Fatalf("expected setup to fail when runtime layout is blocked")
	}
}

func TestSetupFailsWhenServiceUnitPathIsDirectory(t *testing.T) {
	app, _ := newTestApp(t)
	if err := os.MkdirAll(app.Paths.ServiceUnit, 0o755); err != nil {
		t.Fatalf("mkdir blocking service unit path: %v", err)
	}
	if err := app.Setup(); err == nil || !strings.Contains(err.Error(), "is a directory") {
		t.Fatalf("expected service unit write failure, got %v", err)
	}
}

func TestSetupFailsWhenSysctlPathIsDirectory(t *testing.T) {
	app, _ := newTestApp(t)
	if err := os.MkdirAll(app.Paths.SysctlPath, 0o755); err != nil {
		t.Fatalf("mkdir blocking sysctl path: %v", err)
	}
	if err := app.Setup(); err == nil || !strings.Contains(err.Error(), "is a directory") {
		t.Fatalf("expected sysctl write failure, got %v", err)
	}
}

func TestSetupFailsWhenBuiltinRulesPathIsDirectory(t *testing.T) {
	app, _ := newTestApp(t)
	oldGeteuid := geteuid
	geteuid = func() int { return 0 }
	defer func() { geteuid = oldGeteuid }()

	if err := os.MkdirAll(app.Paths.BuiltinRules(), 0o755); err != nil {
		t.Fatalf("mkdir blocking builtin rules path: %v", err)
	}
	if err := app.Setup(); err == nil || !strings.Contains(err.Error(), "is a directory") {
		t.Fatalf("expected builtin rules write failure, got %v", err)
	}
}

func TestSetupFailsWhenManualProviderPathIsDirectory(t *testing.T) {
	app := newTestAppWithEnabledManualNode(t)
	oldGeteuid := geteuid
	geteuid = func() int { return 0 }
	defer func() { geteuid = oldGeteuid }()

	if err := os.MkdirAll(app.Paths.ManualProvider(), 0o755); err != nil {
		t.Fatalf("mkdir blocking manual provider path: %v", err)
	}
	var calls []commandCall
	app.Runner = fakeRunner{runFn: func(name string, args ...string) error {
		calls = append(calls, commandCall{name: name, args: append([]string{}, args...)})
		return nil
	}}
	if err := app.Setup(); err == nil || !strings.Contains(err.Error(), "is a directory") {
		t.Fatalf("expected manual provider write failure, got %v", err)
	}
	assertOnlyCutoverPreflightCalls(t, calls)
}

func TestSetupFailsWhenCustomRulesPathIsDirectory(t *testing.T) {
	app := newTestAppWithEnabledManualNode(t)
	oldGeteuid := geteuid
	geteuid = func() int { return 0 }
	defer func() { geteuid = oldGeteuid }()

	if err := os.MkdirAll(app.Paths.CustomRules(), 0o755); err != nil {
		t.Fatalf("mkdir blocking custom rules path: %v", err)
	}
	var calls []commandCall
	app.Runner = fakeRunner{runFn: func(name string, args ...string) error {
		calls = append(calls, commandCall{name: name, args: append([]string{}, args...)})
		return nil
	}}
	if err := app.Setup(); err == nil || !strings.Contains(err.Error(), "is a directory") {
		t.Fatalf("expected custom rules write failure, got %v", err)
	}
	assertOnlyCutoverPreflightCalls(t, calls)
}

func TestSetupFailsWhenRuntimeConfigPathIsDirectory(t *testing.T) {
	app := newTestAppWithEnabledManualNode(t)
	oldGeteuid := geteuid
	geteuid = func() int { return 0 }
	defer func() { geteuid = oldGeteuid }()

	if err := os.MkdirAll(app.Paths.RuntimeConfig(), 0o755); err != nil {
		t.Fatalf("mkdir blocking runtime config path: %v", err)
	}
	var calls []commandCall
	app.Runner = fakeRunner{runFn: func(name string, args ...string) error {
		calls = append(calls, commandCall{name: name, args: append([]string{}, args...)})
		return nil
	}}
	if err := app.Setup(); err == nil || !strings.Contains(err.Error(), "is a directory") {
		t.Fatalf("expected runtime config write failure, got %v", err)
	}
	assertOnlyCutoverPreflightCalls(t, calls)
}

func TestReadImportInputReturnsAllLinesWhenNotTerminal(t *testing.T) {
	app, _ := newTestApp(t)
	app.Stdin = strings.NewReader("trojan://password@example.org:443?security=tls#one\nsocks5://proxy.example.com:1080#two\n")
	text, err := app.readImportInput()
	if err != nil {
		t.Fatalf("read import input: %v", err)
	}
	if text != "trojan://password@example.org:443?security=tls#one\nsocks5://proxy.example.com:1080#two" {
		t.Fatalf("unexpected import input text: %q", text)
	}
}

func TestReadImportInputKeepsFinalLineWithoutTrailingNewline(t *testing.T) {
	app, _ := newTestApp(t)
	app.Stdin = strings.NewReader("trojan://password@example.org:443?security=tls#no-newline")
	text, err := app.readImportInput()
	if err != nil {
		t.Fatalf("read import input: %v", err)
	}
	if text != "trojan://password@example.org:443?security=tls#no-newline" {
		t.Fatalf("unexpected import input text: %q", text)
	}
}

func TestReadImportInputStopsAtEndWhenTerminal(t *testing.T) {
	app, _ := newTestApp(t)
	oldCheck := terminalCheck
	terminalCheck = func(io.Reader) bool { return true }
	defer func() { terminalCheck = oldCheck }()

	app.Stdin = strings.NewReader("trojan://password@example.org:443?security=tls#one\nend\nsocks5://proxy.example.com:1080#two\n")
	text, err := app.readImportInput()
	if err != nil {
		t.Fatalf("read import input: %v", err)
	}
	if text != "trojan://password@example.org:443?security=tls#one" {
		t.Fatalf("unexpected terminal import text: %q", text)
	}
	if !strings.Contains(app.Stdout.(*bytes.Buffer).String(), "请粘贴节点链接，输入 end 结束") {
		t.Fatalf("expected terminal prompt, got:\n%s", app.Stdout.(*bytes.Buffer).String())
	}
}

func TestStartRendersConfigAndEnablesService(t *testing.T) {
	app, _ := newTestApp(t)
	var calls []commandCall
	app.Runner = fakeRunner{
		runFn: func(name string, args ...string) error {
			calls = append(calls, commandCall{name: name, args: append([]string{}, args...)})
			return nil
		},
		outputFn: func(name string, args ...string) (string, string, error) {
			return "", "", nil
		},
	}
	app.Stdin = strings.NewReader("trojan://password@example.org:443?security=tls#start-node\n")
	if err := app.ImportLinks(); err != nil {
		t.Fatalf("import links: %v", err)
	}
	if err := app.SetNodeEnabled(1, true); err != nil {
		t.Fatalf("enable node: %v", err)
	}
	if err := app.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	if !hasRecordedCall(calls, "systemctl", "enable", "--now", "minimalist.service") {
		t.Fatalf("expected start to enable service, calls=%#v", calls)
	}
	body, err := os.ReadFile(app.Paths.RuntimeConfig())
	if err != nil {
		t.Fatalf("read runtime config: %v", err)
	}
	if !strings.Contains(string(body), "proxy-groups:") {
		t.Fatalf("unexpected runtime config:\n%s", string(body))
	}
}

func TestStartPropagatesRenderConfigFailureWithoutSystemctlCall(t *testing.T) {
	app := newTestAppWithEnabledManualNode(t)
	oldGeteuid := geteuid
	geteuid = func() int { return 0 }
	defer func() { geteuid = oldGeteuid }()

	if err := os.MkdirAll(filepath.Dir(app.Paths.RulesRepoPath()), 0o755); err != nil {
		t.Fatalf("mkdir rules repo dir: %v", err)
	}
	if err := os.WriteFile(app.Paths.RulesRepoPath(), []byte("rulesets: [\n"), 0o640); err != nil {
		t.Fatalf("write invalid manifest: %v", err)
	}
	var calls []commandCall
	app.Runner = fakeRunner{runFn: func(name string, args ...string) error {
		calls = append(calls, commandCall{name: name, args: append([]string{}, args...)})
		return nil
	}}
	if err := app.Start(); err == nil || !strings.Contains(err.Error(), "parse manifest") {
		t.Fatalf("expected render-config failure, got %v", err)
	}
	assertOnlyCutoverPreflightCalls(t, calls)
}

func TestStartFailsWhenManualProviderPathIsDirectory(t *testing.T) {
	app := newTestAppWithEnabledManualNode(t)
	oldGeteuid := geteuid
	geteuid = func() int { return 0 }
	defer func() { geteuid = oldGeteuid }()

	if err := os.MkdirAll(app.Paths.ManualProvider(), 0o755); err != nil {
		t.Fatalf("mkdir blocking manual provider path: %v", err)
	}
	var calls []commandCall
	app.Runner = fakeRunner{runFn: func(name string, args ...string) error {
		calls = append(calls, commandCall{name: name, args: append([]string{}, args...)})
		return nil
	}}
	if err := app.Start(); err == nil || !strings.Contains(err.Error(), "is a directory") {
		t.Fatalf("expected manual provider write failure, got %v", err)
	}
	assertOnlyCutoverPreflightCalls(t, calls)
}

func TestStartFailsWhenRuntimeConfigPathIsDirectory(t *testing.T) {
	app := newTestAppWithEnabledManualNode(t)
	oldGeteuid := geteuid
	geteuid = func() int { return 0 }
	defer func() { geteuid = oldGeteuid }()

	if err := os.MkdirAll(app.Paths.RuntimeConfig(), 0o755); err != nil {
		t.Fatalf("mkdir blocking runtime config path: %v", err)
	}
	var calls []commandCall
	app.Runner = fakeRunner{runFn: func(name string, args ...string) error {
		calls = append(calls, commandCall{name: name, args: append([]string{}, args...)})
		return nil
	}}
	if err := app.Start(); err == nil || !strings.Contains(err.Error(), "is a directory") {
		t.Fatalf("expected runtime config write failure, got %v", err)
	}
	assertOnlyCutoverPreflightCalls(t, calls)
}

func TestStartReturnsRootErrorWhenNotRoot(t *testing.T) {
	app, _ := newTestApp(t)
	oldGeteuid := geteuid
	geteuid = func() int { return 1000 }
	defer func() { geteuid = oldGeteuid }()

	err := app.Start()
	if err == nil || !strings.Contains(err.Error(), "请用 root 运行") {
		t.Fatalf("expected root error, got %v", err)
	}
}

func TestRestartRendersConfigAndRestartsService(t *testing.T) {
	app, _ := newTestApp(t)
	var calls []commandCall
	app.Runner = fakeRunner{
		runFn: func(name string, args ...string) error {
			calls = append(calls, commandCall{name: name, args: append([]string{}, args...)})
			return nil
		},
		outputFn: func(name string, args ...string) (string, string, error) {
			return "", "", nil
		},
	}
	app.Stdin = strings.NewReader("trojan://password@example.org:443?security=tls#restart-node\n")
	if err := app.ImportLinks(); err != nil {
		t.Fatalf("import links: %v", err)
	}
	if err := app.SetNodeEnabled(1, true); err != nil {
		t.Fatalf("enable node: %v", err)
	}
	if err := app.Restart(); err != nil {
		t.Fatalf("restart: %v", err)
	}
	if hasRecordedCall(calls, "systemctl", "enable", "--now", "minimalist.service") {
		t.Fatalf("restart should not enable service, calls=%#v", calls)
	}
	if !hasRecordedCall(calls, "systemctl", "restart", "minimalist.service") {
		t.Fatalf("expected restart to restart service, calls=%#v", calls)
	}
	body, err := os.ReadFile(app.Paths.RuntimeConfig())
	if err != nil {
		t.Fatalf("read runtime config: %v", err)
	}
	if !strings.Contains(string(body), "manual:") {
		t.Fatalf("unexpected runtime config:\n%s", string(body))
	}
}

func TestRestartPropagatesRenderConfigFailureWithoutSystemctlCall(t *testing.T) {
	app := newTestAppWithEnabledManualNode(t)
	oldGeteuid := geteuid
	geteuid = func() int { return 0 }
	defer func() { geteuid = oldGeteuid }()

	if err := os.MkdirAll(filepath.Dir(app.Paths.RulesRepoPath()), 0o755); err != nil {
		t.Fatalf("mkdir rules repo dir: %v", err)
	}
	if err := os.WriteFile(app.Paths.RulesRepoPath(), []byte("rulesets: [\n"), 0o640); err != nil {
		t.Fatalf("write invalid manifest: %v", err)
	}
	var calls []commandCall
	app.Runner = fakeRunner{runFn: func(name string, args ...string) error {
		calls = append(calls, commandCall{name: name, args: append([]string{}, args...)})
		return nil
	}}
	if err := app.Restart(); err == nil || !strings.Contains(err.Error(), "parse manifest") {
		t.Fatalf("expected render-config failure, got %v", err)
	}
	assertOnlyCutoverPreflightCalls(t, calls)
}

func TestRestartFailsWhenCustomRulesPathIsDirectory(t *testing.T) {
	app := newTestAppWithEnabledManualNode(t)
	oldGeteuid := geteuid
	geteuid = func() int { return 0 }
	defer func() { geteuid = oldGeteuid }()

	if err := os.MkdirAll(app.Paths.CustomRules(), 0o755); err != nil {
		t.Fatalf("mkdir blocking custom rules path: %v", err)
	}
	var calls []commandCall
	app.Runner = fakeRunner{runFn: func(name string, args ...string) error {
		calls = append(calls, commandCall{name: name, args: append([]string{}, args...)})
		return nil
	}}
	if err := app.Restart(); err == nil || !strings.Contains(err.Error(), "is a directory") {
		t.Fatalf("expected custom rules write failure, got %v", err)
	}
	assertOnlyCutoverPreflightCalls(t, calls)
}

func TestRestartFailsWhenRuntimeConfigPathIsDirectory(t *testing.T) {
	app := newTestAppWithEnabledManualNode(t)
	oldGeteuid := geteuid
	geteuid = func() int { return 0 }
	defer func() { geteuid = oldGeteuid }()

	if err := os.MkdirAll(app.Paths.RuntimeConfig(), 0o755); err != nil {
		t.Fatalf("mkdir blocking runtime config path: %v", err)
	}
	var calls []commandCall
	app.Runner = fakeRunner{runFn: func(name string, args ...string) error {
		calls = append(calls, commandCall{name: name, args: append([]string{}, args...)})
		return nil
	}}
	if err := app.Restart(); err == nil || !strings.Contains(err.Error(), "is a directory") {
		t.Fatalf("expected runtime config write failure, got %v", err)
	}
	assertOnlyCutoverPreflightCalls(t, calls)
}

func TestRestartReturnsRootErrorWhenNotRoot(t *testing.T) {
	app, _ := newTestApp(t)
	oldGeteuid := geteuid
	geteuid = func() int { return 1000 }
	defer func() { geteuid = oldGeteuid }()

	err := app.Restart()
	if err == nil || !strings.Contains(err.Error(), "请用 root 运行") {
		t.Fatalf("expected root error, got %v", err)
	}
}

func TestStatusFallsBackToConfigModeAndReportsReadySubscriptions(t *testing.T) {
	app, _ := newTestApp(t)
	if err := app.AddSubscription("status-sub", "https://subscription.example.com/status.txt", true); err != nil {
		t.Fatalf("add subscription: %v", err)
	}
	st, err := state.Load(app.Paths.StatePath())
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if len(st.Subscriptions) != 1 {
		t.Fatalf("expected one subscription, got %d", len(st.Subscriptions))
	}
	if err := os.WriteFile(app.Paths.SubscriptionFile(st.Subscriptions[0].ID), []byte("trojan://password@example.org:443?security=tls#status\n"), 0o640); err != nil {
		t.Fatalf("write subscription cache: %v", err)
	}
	if err := app.Status(); err != nil {
		t.Fatalf("status: %v", err)
	}
	output := app.Stdout.(*bytes.Buffer).String()
	for _, needle := range []string{
		"当前模式: rule (config)",
		"服务状态: active=false enabled=false",
		"手动节点: 0",
		"订阅: enabled=1 total=1 ready=1",
	} {
		if !strings.Contains(output, needle) {
			t.Fatalf("missing %q in status output:\n%s", needle, output)
		}
	}
}

func TestStatusCountsOnlyEnabledNonEmptySubscriptionCachesAsReady(t *testing.T) {
	app, _ := newTestApp(t)
	for _, sub := range []struct {
		name string
		url  string
	}{
		{"ready-sub", "https://subscription.example.com/ready.txt"},
		{"empty-sub", "https://subscription.example.com/empty.txt"},
		{"disabled-sub", "https://subscription.example.com/disabled.txt"},
	} {
		if err := app.AddSubscription(sub.name, sub.url, true); err != nil {
			t.Fatalf("add subscription %s: %v", sub.name, err)
		}
	}
	if err := app.SetSubscriptionEnabled(3, false); err != nil {
		t.Fatalf("disable subscription: %v", err)
	}
	st, err := state.Load(app.Paths.StatePath())
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if err := os.MkdirAll(app.Paths.SubscriptionDir(), 0o755); err != nil {
		t.Fatalf("mkdir subscription dir: %v", err)
	}
	if err := os.WriteFile(app.Paths.SubscriptionFile(st.Subscriptions[0].ID), []byte("trojan://password@example.org:443?security=tls#ready\n"), 0o640); err != nil {
		t.Fatalf("write ready cache: %v", err)
	}
	if err := os.WriteFile(app.Paths.SubscriptionFile(st.Subscriptions[1].ID), nil, 0o640); err != nil {
		t.Fatalf("write empty cache: %v", err)
	}
	if err := os.WriteFile(app.Paths.SubscriptionFile(st.Subscriptions[2].ID), []byte("trojan://password@example.org:443?security=tls#disabled\n"), 0o640); err != nil {
		t.Fatalf("write disabled cache: %v", err)
	}
	if err := app.Status(); err != nil {
		t.Fatalf("status: %v", err)
	}
	if !strings.Contains(app.Stdout.(*bytes.Buffer).String(), "订阅: enabled=2 total=3 ready=1") {
		t.Fatalf("unexpected subscription counts:\n%s", app.Stdout.(*bytes.Buffer).String())
	}
}

func TestStatusPrefersRuntimeModeWhenControllerConfigResponds(t *testing.T) {
	app, _ := newTestApp(t)
	app.Client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.Path != "/configs" {
				t.Fatalf("unexpected request path: %s", req.URL.Path)
			}
			return textResponse(http.StatusOK, `{"mode":"global"}`), nil
		}),
	}
	if err := app.Status(); err != nil {
		t.Fatalf("status: %v", err)
	}
	if !strings.Contains(app.Stdout.(*bytes.Buffer).String(), "当前模式: global (runtime)") {
		t.Fatalf("expected runtime mode in status output:\n%s", app.Stdout.(*bytes.Buffer).String())
	}
}

func TestStatusFallsBackToConfigModeWhenControllerConfigIsInvalidJSON(t *testing.T) {
	app, _ := newTestApp(t)
	app.Client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.Path != "/configs" {
				t.Fatalf("unexpected request path: %s", req.URL.Path)
			}
			return textResponse(http.StatusOK, "{"), nil
		}),
	}
	if err := app.Status(); err != nil {
		t.Fatalf("status: %v", err)
	}
	if !strings.Contains(app.Stdout.(*bytes.Buffer).String(), "当前模式: rule (config)") {
		t.Fatalf("expected config mode fallback for invalid json:\n%s", app.Stdout.(*bytes.Buffer).String())
	}
}

func TestStatusReportsManualNodeCountWhenServiceActive(t *testing.T) {
	app, _ := newTestApp(t)
	app.Stdin = strings.NewReader("trojan://password@example.org:443?security=tls#status-manual\n")
	if err := app.ImportLinks(); err != nil {
		t.Fatalf("import links: %v", err)
	}
	if err := app.SetNodeEnabled(1, true); err != nil {
		t.Fatalf("enable manual node: %v", err)
	}
	app.Runner = fakeRunner{
		runFn: func(name string, args ...string) error {
			if name == "systemctl" && len(args) >= 2 && (args[0] == "is-active" || args[0] == "is-enabled") {
				return nil
			}
			return nil
		},
		outputFn: func(name string, args ...string) (string, string, error) {
			return "", "", nil
		},
	}
	if err := app.Status(); err != nil {
		t.Fatalf("status: %v", err)
	}
	output := app.Stdout.(*bytes.Buffer).String()
	for _, needle := range []string{
		"服务状态: active=true enabled=true",
		"手动节点: 1",
		"订阅: enabled=0 total=0 ready=0",
	} {
		if !strings.Contains(output, needle) {
			t.Fatalf("missing %q in status output:\n%s", needle, output)
		}
	}
}

func TestHealthcheckReportsControllerSummary(t *testing.T) {
	app, _ := newTestApp(t)
	app.Client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.Path != "/version" {
				t.Fatalf("unexpected request path: %s", req.URL.Path)
			}
			if got := req.Header.Get("Authorization"); got == "" {
				t.Fatalf("expected authorization header")
			}
			return textResponse(http.StatusOK, "Mihomo Meta v1.0.0\n"), nil
		}),
	}
	if err := app.Healthcheck(); err != nil {
		t.Fatalf("healthcheck: %v", err)
	}
	output := app.Stdout.(*bytes.Buffer).String()
	for _, needle := range []string{
		"mixed-port=7890",
		"tproxy-port=7893",
		"dns-port=1053",
		"controller-port=19090",
		"Mihomo Meta v1.0.0",
	} {
		if !strings.Contains(output, needle) {
			t.Fatalf("missing %q in healthcheck output:\n%s", needle, output)
		}
	}
}

func TestHealthcheckReportsControllerErrorWhenUnavailable(t *testing.T) {
	app, _ := newTestApp(t)
	if err := app.Healthcheck(); err != nil {
		t.Fatalf("healthcheck: %v", err)
	}
	output := app.Stdout.(*bytes.Buffer).String()
	if !strings.Contains(output, "controller: Get") || !strings.Contains(output, "unavailable") {
		t.Fatalf("expected controller error in healthcheck output:\n%s", output)
	}
}

func TestRuntimeAuditCountsAlertsAndReportsRuntimeSummary(t *testing.T) {
	app, _ := newTestApp(t)
	app.Runner = fakeRunner{
		runFn: func(name string, args ...string) error {
			if name == "systemctl" && len(args) >= 2 && args[0] == "is-active" {
				return nil
			}
			if name == "systemctl" && len(args) >= 2 && args[0] == "is-enabled" {
				return nil
			}
			return nil
		},
		outputFn: func(name string, args ...string) (string, string, error) {
			if name == "journalctl" {
				return "INFO booted\nWARN slow-provider\nERROR dial failed\n", "", nil
			}
			return "", "", errors.New("unavailable")
		},
	}
	app.Client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Path {
			case "/configs":
				return textResponse(http.StatusOK, `{"mode":"global"}`), nil
			case "/version":
				return textResponse(http.StatusOK, "Mihomo Meta v1.0.1\n"), nil
			default:
				t.Fatalf("unexpected request path: %s", req.URL.Path)
				return nil, nil
			}
		}),
	}
	if err := app.RuntimeAudit(); err != nil {
		t.Fatalf("runtime audit: %v", err)
	}
	output := app.Stdout.(*bytes.Buffer).String()
	for _, needle := range []string{
		"当前模式: global (runtime)",
		"服务状态: active=true enabled=true",
		"alerts: warn=1 error=1",
		"providers-ready=false",
		"runtime: Mihomo Meta v1.0.1",
	} {
		if !strings.Contains(output, needle) {
			t.Fatalf("missing %q in runtime audit output:\n%s", needle, output)
		}
	}
}

func TestRuntimeAuditOmitsRuntimeSummaryWhenControllerUnavailable(t *testing.T) {
	app, _ := newTestApp(t)
	app.Runner = fakeRunner{
		runFn: func(name string, args ...string) error {
			if name == "systemctl" && len(args) >= 2 && (args[0] == "is-active" || args[0] == "is-enabled") {
				return nil
			}
			return nil
		},
		outputFn: func(name string, args ...string) (string, string, error) {
			if name == "journalctl" {
				return "WARN retrying\n", "", nil
			}
			return "", "", errors.New("unavailable")
		},
	}
	if err := app.RuntimeAudit(); err != nil {
		t.Fatalf("runtime audit: %v", err)
	}
	output := app.Stdout.(*bytes.Buffer).String()
	if !strings.Contains(output, "alerts: warn=1 error=0") {
		t.Fatalf("expected alert count in runtime audit output:\n%s", output)
	}
	if strings.Contains(output, "runtime: ") {
		t.Fatalf("did not expect runtime summary when controller is unavailable:\n%s", output)
	}
}

func TestRuntimeAuditKeepsLocalSummaryWhenJournalctlFails(t *testing.T) {
	app, _ := newTestApp(t)
	app.Runner = fakeRunner{
		runFn: func(name string, args ...string) error {
			if name == "systemctl" && len(args) >= 2 && (args[0] == "is-active" || args[0] == "is-enabled") {
				return nil
			}
			return nil
		},
		outputFn: func(name string, args ...string) (string, string, error) {
			if name == "journalctl" {
				return "", "", errors.New("journal unavailable")
			}
			return "", "", errors.New("unavailable")
		},
	}
	if err := app.RuntimeAudit(); err != nil {
		t.Fatalf("runtime audit: %v", err)
	}
	output := app.Stdout.(*bytes.Buffer).String()
	for _, needle := range []string{
		"服务状态: active=true enabled=true",
		"alerts: warn=0 error=0",
		"providers-ready=false",
		"cutover-preflight:",
	} {
		if !strings.Contains(output, needle) {
			t.Fatalf("missing %q in runtime audit output:\n%s", needle, output)
		}
	}
	if strings.Contains(output, "journal unavailable") {
		t.Fatalf("journalctl failure should not be printed as a runtime alert:\n%s", output)
	}
}

func TestRuntimeAuditReportsLegacyLiveCutoverRisk(t *testing.T) {
	app, root := newTestApp(t)
	oldLegacy := legacyLiveInstall
	legacyLiveInstall = struct {
		BinPath   string
		ConfigDir string
	}{
		BinPath:   filepath.Join(root, "usr", "local", "bin", "mihomo"),
		ConfigDir: filepath.Join(root, "etc", "mihomo"),
	}
	defer func() { legacyLiveInstall = oldLegacy }()
	if err := os.MkdirAll(filepath.Dir(legacyLiveInstall.BinPath), 0o755); err != nil {
		t.Fatalf("mkdir legacy bin dir: %v", err)
	}
	if err := os.WriteFile(legacyLiveInstall.BinPath, []byte("#!/usr/bin/env bash\n"), 0o755); err != nil {
		t.Fatalf("write legacy bin: %v", err)
	}
	if err := os.MkdirAll(legacyLiveInstall.ConfigDir, 0o750); err != nil {
		t.Fatalf("mkdir legacy config dir: %v", err)
	}
	var calls []commandCall
	app.Runner = fakeRunner{
		runFn: func(name string, args ...string) error {
			calls = append(calls, commandCall{name: name, args: append([]string{}, args...)})
			if name == "systemctl" && len(args) >= 3 && args[2] == "mihomo.service" {
				return nil
			}
			return errors.New("inactive")
		},
		outputFn: func(name string, args ...string) (string, string, error) {
			if name == "journalctl" {
				return "", "", nil
			}
			return "", "", errors.New("unavailable")
		},
	}
	if err := app.RuntimeAudit(); err != nil {
		t.Fatalf("runtime audit: %v", err)
	}
	output := app.Stdout.(*bytes.Buffer).String()
	for _, needle := range []string{
		"cutover-preflight: legacy_service_active=true legacy_service_enabled=true legacy_bin=true legacy_config_dir=true minimalist_service_active=false minimalist_service_enabled=false minimalist_unit=false minimalist_bin=false",
		"cutover-warning: legacy live install detected",
		"cutover-note: legacy mihomo and minimalist use the same MIHOMO_* chains plus 0x2333/table 233 defaults",
		"cutover-ready=false",
	} {
		if !strings.Contains(output, needle) {
			t.Fatalf("missing %q in runtime audit output:\n%s", needle, output)
		}
	}
	for _, call := range calls {
		if call.name == "systemctl" && len(call.args) > 0 && (call.args[0] == "stop" || call.args[0] == "restart" || call.args[0] == "enable") {
			t.Fatalf("runtime audit must stay read-only, got call %#v", call)
		}
	}
}

func TestCutoverPreflightIsReadOnly(t *testing.T) {
	app, root := newTestApp(t)
	oldLegacy := legacyLiveInstall
	legacyLiveInstall = struct {
		BinPath   string
		ConfigDir string
	}{
		BinPath:   filepath.Join(root, "legacy", "mihomo"),
		ConfigDir: filepath.Join(root, "legacy", "etc"),
	}
	defer func() { legacyLiveInstall = oldLegacy }()
	var calls []commandCall
	app.Runner = fakeRunner{
		runFn: func(name string, args ...string) error {
			calls = append(calls, commandCall{name: name, args: append([]string{}, args...)})
			return errors.New("inactive")
		},
	}
	if err := app.CutoverPreflight(); err != nil {
		t.Fatalf("cutover preflight: %v", err)
	}
	output := app.Stdout.(*bytes.Buffer).String()
	if !strings.Contains(output, "cutover-preflight:") || !strings.Contains(output, "cutover-ready=true") {
		t.Fatalf("unexpected cutover preflight output:\n%s", output)
	}
	if _, err := os.Stat(app.Paths.ConfigPath()); !os.IsNotExist(err) {
		t.Fatalf("cutover preflight must not create config, stat err=%v", err)
	}
	for _, call := range calls {
		if call.name == "systemctl" && len(call.args) > 0 && (call.args[0] == "stop" || call.args[0] == "restart" || call.args[0] == "enable") {
			t.Fatalf("cutover preflight must stay read-only, got call %#v", call)
		}
	}
}

func TestCutoverPlanReportsCurrentState(t *testing.T) {
	tests := []struct {
		name         string
		legacyAssets bool
		runFn        func(name string, args ...string) error
		needles      []string
	}{
		{
			name: "legacy-live",
			runFn: func(name string, args ...string) error {
				if name == "systemctl" && len(args) >= 3 && args[2] == "mihomo.service" {
					return nil
				}
				return errors.New("inactive")
			},
			needles: []string{
				"cutover-plan: legacy_live=true minimalist_service_live=false cutover_ready=false",
				"next-action: prepare-minimalist-inputs",
				"maintenance-window: disable --now mihomo.service",
			},
		},
		{
			name:         "legacy-stopped",
			legacyAssets: true,
			runFn: func(name string, args ...string) error {
				return errors.New("inactive")
			},
			needles: []string{
				"cutover-plan: legacy_live=false minimalist_service_live=false cutover_ready=true",
				"next-action: run-minimalist-setup",
				"rollback: disable --now minimalist.service; enable --now mihomo.service",
			},
		},
		{
			name: "minimalist-active",
			runFn: func(name string, args ...string) error {
				if name == "systemctl" && len(args) >= 3 && args[2] == "minimalist.service" {
					return nil
				}
				return errors.New("inactive")
			},
			needles: []string{
				"cutover-plan: legacy_live=false minimalist_service_live=true cutover_ready=true",
				"next-action: validate-minimalist",
				"rollback: unavailable; legacy mihomo assets are not present",
			},
		},
		{
			name: "legacy-and-minimalist-live",
			runFn: func(name string, args ...string) error {
				if name == "systemctl" && len(args) >= 3 && (args[2] == "mihomo.service" || args[2] == "minimalist.service") {
					return nil
				}
				return errors.New("inactive")
			},
			needles: []string{
				"cutover-plan: legacy_live=true minimalist_service_live=true cutover_ready=true",
				"next-action: validate-minimalist",
				"rollback: disable --now minimalist.service; enable --now mihomo.service",
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			app, root := newTestApp(t)
			oldLegacy := legacyLiveInstall
			legacyLiveInstall = struct {
				BinPath   string
				ConfigDir string
			}{
				BinPath:   filepath.Join(root, "legacy", "mihomo"),
				ConfigDir: filepath.Join(root, "legacy", "etc"),
			}
			defer func() { legacyLiveInstall = oldLegacy }()
			if tc.legacyAssets {
				if err := os.MkdirAll(legacyLiveInstall.ConfigDir, 0o755); err != nil {
					t.Fatalf("create legacy config dir: %v", err)
				}
				if err := os.MkdirAll(filepath.Dir(legacyLiveInstall.BinPath), 0o755); err != nil {
					t.Fatalf("create legacy bin dir: %v", err)
				}
				if err := os.WriteFile(legacyLiveInstall.BinPath, []byte("legacy"), 0o755); err != nil {
					t.Fatalf("create legacy bin: %v", err)
				}
			}
			var calls []commandCall
			app.Runner = fakeRunner{
				runFn: func(name string, args ...string) error {
					calls = append(calls, commandCall{name: name, args: append([]string{}, args...)})
					return tc.runFn(name, args...)
				},
			}
			if err := app.CutoverPlan(); err != nil {
				t.Fatalf("cutover plan: %v", err)
			}
			output := app.Stdout.(*bytes.Buffer).String()
			for _, needle := range tc.needles {
				if !strings.Contains(output, needle) {
					t.Fatalf("missing %q in cutover plan output:\n%s", needle, output)
				}
			}
			if _, err := os.Stat(app.Paths.ConfigPath()); !os.IsNotExist(err) {
				t.Fatalf("cutover plan must not create config, stat err=%v", err)
			}
			assertOnlyCutoverPreflightCalls(t, calls)
		})
	}
}

func TestCutoverReadyStates(t *testing.T) {
	t.Run("normal-empty-env", func(t *testing.T) {
		app, _ := newTestApp(t)
		status := app.cutoverPreflightStatus()
		if !status.Ready() {
			t.Fatalf("expected empty env to be ready, got %#v", status)
		}
		if err := app.ensureCutoverReady(); err != nil {
			t.Fatalf("ensureCutoverReady: %v", err)
		}
	})

	t.Run("legacy-live-blocks", func(t *testing.T) {
		app, root := newTestApp(t)
		oldLegacy := legacyLiveInstall
		legacyLiveInstall = struct {
			BinPath   string
			ConfigDir string
		}{
			BinPath:   filepath.Join(root, "usr", "local", "bin", "mihomo"),
			ConfigDir: filepath.Join(root, "etc", "mihomo"),
		}
		defer func() { legacyLiveInstall = oldLegacy }()
		if err := os.MkdirAll(filepath.Dir(legacyLiveInstall.BinPath), 0o755); err != nil {
			t.Fatalf("mkdir legacy bin dir: %v", err)
		}
		if err := os.WriteFile(legacyLiveInstall.BinPath, []byte("#!/usr/bin/env bash\n"), 0o755); err != nil {
			t.Fatalf("write legacy bin: %v", err)
		}
		if err := os.MkdirAll(legacyLiveInstall.ConfigDir, 0o750); err != nil {
			t.Fatalf("mkdir legacy config dir: %v", err)
		}
		app.Runner = fakeRunner{
			runFn: func(name string, args ...string) error {
				if name == "systemctl" && len(args) >= 3 && args[0] == "is-active" && args[2] == "mihomo.service" {
					return nil
				}
				return errors.New("inactive")
			},
		}
		status := app.cutoverPreflightStatus()
		if status.Ready() {
			t.Fatalf("expected legacy live install to be blocked, got %#v", status)
		}
		if err := app.ensureCutoverReady(); err == nil || !strings.Contains(err.Error(), "cutover blocked") {
			t.Fatalf("expected cutover blocked error, got %v", err)
		}
	})

	t.Run("legacy-live-with-minimalist-bin-still-blocks", func(t *testing.T) {
		app, root := newTestApp(t)
		oldLegacy := legacyLiveInstall
		legacyLiveInstall = struct {
			BinPath   string
			ConfigDir string
		}{
			BinPath:   filepath.Join(root, "usr", "local", "bin", "mihomo"),
			ConfigDir: filepath.Join(root, "etc", "mihomo"),
		}
		defer func() { legacyLiveInstall = oldLegacy }()
		if err := os.MkdirAll(filepath.Dir(app.Paths.BinPath), 0o755); err != nil {
			t.Fatalf("mkdir minimalist bin dir: %v", err)
		}
		if err := os.WriteFile(app.Paths.BinPath, []byte("#!/usr/bin/env bash\n"), 0o755); err != nil {
			t.Fatalf("write minimalist bin: %v", err)
		}
		app.Runner = fakeRunner{
			runFn: func(name string, args ...string) error {
				if name == "systemctl" && len(args) >= 3 && args[0] == "is-active" && args[2] == "mihomo.service" {
					return nil
				}
				return errors.New("inactive")
			},
		}
		status := app.cutoverPreflightStatus()
		if status.Ready() {
			t.Fatalf("expected legacy live install with only minimalist bin to be blocked, got %#v", status)
		}
		if err := app.ensureCutoverReady(); err == nil || !strings.Contains(err.Error(), "cutover blocked") {
			t.Fatalf("expected cutover blocked error, got %v", err)
		}
	})

	t.Run("minimalist-installed-without-legacy-live-allows", func(t *testing.T) {
		app, root := newTestApp(t)
		oldLegacy := legacyLiveInstall
		legacyLiveInstall = struct {
			BinPath   string
			ConfigDir string
		}{
			BinPath:   filepath.Join(root, "usr", "local", "bin", "mihomo"),
			ConfigDir: filepath.Join(root, "etc", "mihomo"),
		}
		defer func() { legacyLiveInstall = oldLegacy }()
		if err := os.MkdirAll(filepath.Dir(app.Paths.BinPath), 0o755); err != nil {
			t.Fatalf("mkdir minimalist bin dir: %v", err)
		}
		if err := os.WriteFile(app.Paths.BinPath, []byte("#!/usr/bin/env bash\n"), 0o755); err != nil {
			t.Fatalf("write minimalist bin: %v", err)
		}
		status := app.cutoverPreflightStatus()
		if !status.Ready() {
			t.Fatalf("expected minimalist install without legacy live service to be allowed, got %#v", status)
		}
		if err := app.ensureCutoverReady(); err != nil {
			t.Fatalf("ensureCutoverReady: %v", err)
		}
	})
}

func TestHighRiskCommandsBlockOnLegacyLiveInstall(t *testing.T) {
	oldGeteuid := geteuid
	geteuid = func() int { return 0 }
	defer func() { geteuid = oldGeteuid }()

	tests := []struct {
		name   string
		invoke func(*App) error
	}{
		{"setup", func(app *App) error { return app.Setup() }},
		{"start", func(app *App) error { return app.Start() }},
		{"restart", func(app *App) error { return app.Restart() }},
		{"apply-rules", func(app *App) error { return app.ApplyRules() }},
		{"clear-rules", func(app *App) error { return app.ClearRules() }},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			app, root := newTestApp(t)
			oldLegacy := legacyLiveInstall
			legacyLiveInstall = struct {
				BinPath   string
				ConfigDir string
			}{
				BinPath:   filepath.Join(root, "usr", "local", "bin", "mihomo"),
				ConfigDir: filepath.Join(root, "etc", "mihomo"),
			}
			defer func() { legacyLiveInstall = oldLegacy }()
			if err := os.MkdirAll(filepath.Dir(legacyLiveInstall.BinPath), 0o755); err != nil {
				t.Fatalf("mkdir legacy bin dir: %v", err)
			}
			if err := os.WriteFile(legacyLiveInstall.BinPath, []byte("#!/usr/bin/env bash\n"), 0o755); err != nil {
				t.Fatalf("write legacy bin: %v", err)
			}
			if err := os.MkdirAll(legacyLiveInstall.ConfigDir, 0o750); err != nil {
				t.Fatalf("mkdir legacy config dir: %v", err)
			}
			var calls []commandCall
			app.Runner = fakeRunner{
				runFn: func(name string, args ...string) error {
					calls = append(calls, commandCall{name: name, args: append([]string{}, args...)})
					if name == "systemctl" && len(args) >= 3 && args[0] == "is-active" && args[2] == "mihomo.service" {
						return nil
					}
					return errors.New("inactive")
				},
			}
			err := tc.invoke(app)
			if err == nil || !strings.Contains(err.Error(), "cutover blocked") {
				t.Fatalf("expected cutover blocked error, got %v", err)
			}
			if _, statErr := os.Stat(app.Paths.ConfigPath()); !os.IsNotExist(statErr) {
				t.Fatalf("%s must not create config, stat err=%v", tc.name, statErr)
			}
			for _, call := range calls {
				if call.name == "iptables" || call.name == "ip" {
					t.Fatalf("%s must not touch networking commands, got call %#v", tc.name, call)
				}
				if call.name == "systemctl" && len(call.args) > 0 && (call.args[0] == "enable" || call.args[0] == "restart" || call.args[0] == "stop") {
					t.Fatalf("%s must not perform service mutations, got call %#v", tc.name, call)
				}
				if call.name == "sysctl" {
					t.Fatalf("%s must not touch sysctl, got call %#v", tc.name, call)
				}
			}
		})
	}
}

func TestShowSecretPrintsConfiguredSecret(t *testing.T) {
	app, _ := newTestApp(t)
	cfg, err := config.Ensure(app.Paths.ConfigPath())
	if err != nil {
		t.Fatalf("ensure config: %v", err)
	}
	cfg.Controller.Secret = "app-secret"
	if err := config.Save(app.Paths.ConfigPath(), cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	if err := app.ShowSecret(); err != nil {
		t.Fatalf("show secret: %v", err)
	}
	if !strings.Contains(app.Stdout.(*bytes.Buffer).String(), "app-secret") {
		t.Fatalf("unexpected show secret output:\n%s", app.Stdout.(*bytes.Buffer).String())
	}
}

func TestInstallSelfWritesBinaryConfigStateAndRulesRepo(t *testing.T) {
	app, _ := newTestApp(t)
	err := app.InstallSelf()
	if os.Geteuid() != 0 {
		if err == nil || !strings.Contains(err.Error(), "请用 root 运行") {
			t.Fatalf("expected root error, got %v", err)
		}
		return
	}
	if err != nil {
		t.Fatalf("install self: %v", err)
	}
	for _, path := range []string{
		app.Paths.BinPath,
		app.Paths.ConfigPath(),
		app.Paths.StatePath(),
		app.Paths.RulesRepoPath(),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected %s to exist: %v", path, err)
		}
	}
	if !strings.Contains(app.Stdout.(*bytes.Buffer).String(), "已安装 minimalist 到 "+app.Paths.BinPath) {
		t.Fatalf("unexpected install-self output:\n%s", app.Stdout.(*bytes.Buffer).String())
	}
}

func TestStopRunsSystemctlStop(t *testing.T) {
	app, _ := newTestApp(t)
	var calls []commandCall
	app.Runner = fakeRunner{
		runFn: func(name string, args ...string) error {
			calls = append(calls, commandCall{name: name, args: append([]string{}, args...)})
			return nil
		},
	}
	err := app.Stop()
	if os.Geteuid() != 0 {
		if err == nil || !strings.Contains(err.Error(), "请用 root 运行") {
			t.Fatalf("expected root error, got %v", err)
		}
		return
	}
	if err != nil {
		t.Fatalf("stop: %v", err)
	}
	if !hasRecordedCall(calls, "systemctl", "stop", "minimalist.service") {
		t.Fatalf("expected systemctl stop call, got %#v", calls)
	}
}

func TestStopReturnsRootErrorWhenNotRoot(t *testing.T) {
	app, _ := newTestApp(t)
	oldGeteuid := geteuid
	geteuid = func() int { return 1000 }
	defer func() { geteuid = oldGeteuid }()

	err := app.Stop()
	if err == nil || !strings.Contains(err.Error(), "请用 root 运行") {
		t.Fatalf("expected root error, got %v", err)
	}
}

func TestRenderConfigRejectsInvalidPersistedRuleTarget(t *testing.T) {
	app, _ := newTestApp(t)
	st := state.Empty()
	st.Rules = []state.Rule{{ID: "rule-1", Kind: "domain", Pattern: "example.com", Target: "AUTO"}}
	if err := state.Save(app.Paths.StatePath(), st); err != nil {
		t.Fatalf("save state: %v", err)
	}
	err := app.RenderConfig()
	if err == nil || !strings.Contains(err.Error(), "无效规则目标: AUTO") {
		t.Fatalf("expected invalid target error, got %v", err)
	}
}

func TestRenderConfigRejectsPersistedSubscriptionNodeTarget(t *testing.T) {
	app, _ := newTestApp(t)
	st := state.Empty()
	st.Nodes = []state.Node{{
		ID:         "sub-node-1",
		Name:       "sub-only-node",
		Enabled:    true,
		URI:        "trojan://password@example.org:443?security=tls#sub-only-node",
		ImportedAt: state.NowISO(),
		Source:     state.Source{Kind: "subscription", ID: "sub-1"},
	}}
	st.Rules = []state.Rule{{ID: "rule-1", Kind: "domain", Pattern: "example.com", Target: "sub-only-node"}}
	if err := state.Save(app.Paths.StatePath(), st); err != nil {
		t.Fatalf("save state: %v", err)
	}
	err := app.RenderConfig()
	if err == nil || !strings.Contains(err.Error(), "无效规则目标: sub-only-node") {
		t.Fatalf("expected subscription target validation error, got %v", err)
	}
	if _, statErr := os.Stat(app.Paths.RuntimeConfig()); !os.IsNotExist(statErr) {
		t.Fatalf("render-config should not write runtime config after validation failure, stat err=%v", statErr)
	}
}

func TestRenameNodeRewritesRuleAndACLTargets(t *testing.T) {
	app, _ := newTestApp(t)
	app.Stdin = strings.NewReader("trojan://password@example.org:443?security=tls#rename-me\n")
	if err := app.ImportLinks(); err != nil {
		t.Fatalf("import links: %v", err)
	}
	if err := app.AddRule(false, "domain", "example.com", "rename-me"); err != nil {
		t.Fatalf("add rule: %v", err)
	}
	if err := app.AddRule(true, "src-cidr", "192.168.2.10/32", "rename-me"); err != nil {
		t.Fatalf("add acl: %v", err)
	}
	if err := app.RenameNode(1, "renamed-node"); err != nil {
		t.Fatalf("rename node: %v", err)
	}
	st, err := state.Load(app.Paths.StatePath())
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	for _, needle := range []string{
		st.Nodes[0].Name,
		st.Rules[0].Target,
		st.ACL[0].Target,
	} {
		if needle != "renamed-node" {
			t.Fatalf("expected renamed target, got state=%+v", st)
		}
	}
}

func TestRenameNodeRejectsEmptyNameBeforePersisting(t *testing.T) {
	app, _ := newTestApp(t)
	app.Stdin = strings.NewReader("trojan://password@example.org:443?security=tls#keep-name\n")
	if err := app.ImportLinks(); err != nil {
		t.Fatalf("import links: %v", err)
	}
	err := app.RenameNode(1, "   ")
	if err == nil || !strings.Contains(err.Error(), "node name is empty") {
		t.Fatalf("expected empty node name error, got %v", err)
	}
	st, err := state.Load(app.Paths.StatePath())
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if len(st.Nodes) != 1 || st.Nodes[0].Name != "keep-name" {
		t.Fatalf("expected original node name to remain, got %+v", st.Nodes)
	}
}

func TestRenameNodeTrimsNameBeforePersisting(t *testing.T) {
	app, _ := newTestApp(t)
	app.Stdin = strings.NewReader("trojan://password@example.org:443?security=tls#rename-spaces\n")
	if err := app.ImportLinks(); err != nil {
		t.Fatalf("import links: %v", err)
	}
	if err := app.AddRule(false, "domain", "example.com", "rename-spaces"); err != nil {
		t.Fatalf("add rule: %v", err)
	}
	if err := app.RenameNode(1, " renamed-trimmed "); err != nil {
		t.Fatalf("rename node: %v", err)
	}
	st, err := state.Load(app.Paths.StatePath())
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if st.Nodes[0].Name != "renamed-trimmed" || st.Rules[0].Target != "renamed-trimmed" {
		t.Fatalf("expected trimmed node name and rule target, got %+v", st)
	}
}

func TestRenameNodeRejectsSubscriptionNode(t *testing.T) {
	app, _ := newTestApp(t)
	app.Client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return textResponse(http.StatusOK, "trojan://password@example.org:443?security=tls#sub-node\n"), nil
		}),
	}
	if err := app.AddSubscription("rename-sub", "https://subscription.example.com/rename.txt", true); err != nil {
		t.Fatalf("add subscription: %v", err)
	}
	if err := app.UpdateSubscriptions(); err != nil {
		t.Fatalf("update subscriptions: %v", err)
	}
	err := app.RenameNode(1, "should-fail")
	if err == nil || !strings.Contains(err.Error(), "subscription node is provider-managed") {
		t.Fatalf("expected provider-managed error, got %v", err)
	}
}

func TestRemoveNodeRejectsSubscriptionNode(t *testing.T) {
	app, _ := newTestApp(t)
	app.Client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return textResponse(http.StatusOK, "trojan://password@example.org:443?security=tls#sub-node\n"), nil
		}),
	}
	if err := app.AddSubscription("remove-node-sub", "https://subscription.example.com/remove-node.txt", true); err != nil {
		t.Fatalf("add subscription: %v", err)
	}
	if err := app.UpdateSubscriptions(); err != nil {
		t.Fatalf("update subscriptions: %v", err)
	}
	err := app.RemoveNode(1)
	if err == nil || !strings.Contains(err.Error(), "subscription node is provider-managed") {
		t.Fatalf("expected provider-managed error, got %v", err)
	}
	st, err := state.Load(app.Paths.StatePath())
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if len(st.Nodes) != 1 || st.Nodes[0].Source.Kind != "subscription" {
		t.Fatalf("expected subscription node to remain, got %+v", st.Nodes)
	}
}

func TestAddSubscriptionUpdatesExistingURLInPlace(t *testing.T) {
	app, _ := newTestApp(t)
	url := "https://subscription.example.com/shared.txt"
	if err := app.AddSubscription("first-name", url, true); err != nil {
		t.Fatalf("add subscription: %v", err)
	}
	if err := app.AddSubscription("renamed-sub", url, false); err != nil {
		t.Fatalf("update subscription: %v", err)
	}
	st, err := state.Load(app.Paths.StatePath())
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if len(st.Subscriptions) != 1 {
		t.Fatalf("expected one subscription, got %d", len(st.Subscriptions))
	}
	if st.Subscriptions[0].Name != "renamed-sub" || st.Subscriptions[0].Enabled {
		t.Fatalf("expected updated subscription fields, got %+v", st.Subscriptions[0])
	}
}

func TestAddSubscriptionRejectsEmptyNameOrURLBeforePersisting(t *testing.T) {
	app, _ := newTestApp(t)
	tests := []struct {
		name string
		url  string
		want string
	}{
		{name: "   ", url: "https://subscription.example.com/blank-name.txt", want: "subscription name is empty"},
		{name: "blank-url", url: "   ", want: "subscription url is empty"},
	}
	for _, tc := range tests {
		err := app.AddSubscription(tc.name, tc.url, true)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("expected %q, got %v", tc.want, err)
		}
	}
	st, err := state.Load(app.Paths.StatePath())
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if len(st.Subscriptions) != 0 {
		t.Fatalf("expected no invalid subscriptions persisted, got %+v", st.Subscriptions)
	}
}

func TestAddSubscriptionTrimsNameAndURLBeforePersisting(t *testing.T) {
	app, _ := newTestApp(t)
	url := "https://subscription.example.com/trimmed.txt"
	if err := app.AddSubscription(" first-name ", " "+url+" ", true); err != nil {
		t.Fatalf("add subscription: %v", err)
	}
	if err := app.AddSubscription(" renamed-sub ", url, false); err != nil {
		t.Fatalf("update subscription: %v", err)
	}
	st, err := state.Load(app.Paths.StatePath())
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if len(st.Subscriptions) != 1 {
		t.Fatalf("expected trimmed URL to dedupe to one subscription, got %+v", st.Subscriptions)
	}
	sub := st.Subscriptions[0]
	if sub.Name != "renamed-sub" || sub.URL != url || sub.Enabled {
		t.Fatalf("expected trimmed updated subscription, got %+v", sub)
	}
}

func TestSetSubscriptionDisabledPurgesSubscriptionNodes(t *testing.T) {
	app, _ := newTestApp(t)
	app.Client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return textResponse(http.StatusOK, "trojan://password@example.org:443?security=tls#sub-purge\n"), nil
		}),
	}
	if err := app.AddSubscription("purge-sub", "https://subscription.example.com/purge.txt", true); err != nil {
		t.Fatalf("add subscription: %v", err)
	}
	if err := app.UpdateSubscriptions(); err != nil {
		t.Fatalf("update subscriptions: %v", err)
	}
	if err := app.SetSubscriptionEnabled(1, false); err != nil {
		t.Fatalf("disable subscription: %v", err)
	}
	st, err := state.Load(app.Paths.StatePath())
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if st.Subscriptions[0].Enabled {
		t.Fatalf("expected subscription to be disabled")
	}
	for _, node := range st.Nodes {
		if node.Source.Kind == "subscription" {
			t.Fatalf("expected subscription nodes to be purged, got %+v", st.Nodes)
		}
	}
}

func TestRemoveSubscriptionDeletesCacheAndNodes(t *testing.T) {
	app, _ := newTestApp(t)
	app.Client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return textResponse(http.StatusOK, "trojan://password@example.org:443?security=tls#sub-remove\n"), nil
		}),
	}
	if err := app.AddSubscription("remove-sub", "https://subscription.example.com/remove.txt", true); err != nil {
		t.Fatalf("add subscription: %v", err)
	}
	if err := app.UpdateSubscriptions(); err != nil {
		t.Fatalf("update subscriptions: %v", err)
	}
	st, err := state.Load(app.Paths.StatePath())
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	cachePath := app.Paths.SubscriptionFile(st.Subscriptions[0].ID)
	if _, err := os.Stat(cachePath); err != nil {
		t.Fatalf("expected cache file before remove: %v", err)
	}
	if err := app.RemoveSubscription(1); err != nil {
		t.Fatalf("remove subscription: %v", err)
	}
	st, err = state.Load(app.Paths.StatePath())
	if err != nil {
		t.Fatalf("reload state: %v", err)
	}
	if len(st.Subscriptions) != 0 || len(st.Nodes) != 0 {
		t.Fatalf("expected subscription and nodes removed, got %+v", st)
	}
	if _, err := os.Stat(cachePath); !os.IsNotExist(err) {
		t.Fatalf("expected cache file removal, got %v", err)
	}
}

func TestRemoveSubscriptionIgnoresMissingCache(t *testing.T) {
	app, _ := newTestApp(t)
	if err := app.AddSubscription("missing-cache", "https://subscription.example.com/missing.txt", true); err != nil {
		t.Fatalf("add subscription: %v", err)
	}
	if err := app.RemoveSubscription(1); err != nil {
		t.Fatalf("remove subscription with missing cache: %v", err)
	}
	st, err := state.Load(app.Paths.StatePath())
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if len(st.Subscriptions) != 0 {
		t.Fatalf("expected subscription removed, got %+v", st.Subscriptions)
	}
}

func TestRemoveSubscriptionReturnsCacheRemovalFailure(t *testing.T) {
	app, _ := newTestApp(t)
	app.Client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return textResponse(http.StatusOK, "trojan://password@example.org:443?security=tls#remove-fail-node\n"), nil
		}),
	}
	if err := app.AddSubscription("remove-fail", "https://subscription.example.com/remove-fail.txt", true); err != nil {
		t.Fatalf("add subscription: %v", err)
	}
	if err := app.UpdateSubscriptions(); err != nil {
		t.Fatalf("update subscriptions: %v", err)
	}
	st, err := state.Load(app.Paths.StatePath())
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	cachePath := app.Paths.SubscriptionFile(st.Subscriptions[0].ID)
	if err := os.Remove(cachePath); err != nil {
		t.Fatalf("remove cache file before directory replacement: %v", err)
	}
	if err := os.MkdirAll(cachePath, 0o755); err != nil {
		t.Fatalf("mkdir blocking cache path: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cachePath, "keep"), []byte("blocked"), 0o640); err != nil {
		t.Fatalf("write blocking cache child: %v", err)
	}
	err = app.RemoveSubscription(1)
	if err == nil || !strings.Contains(err.Error(), "directory not empty") {
		t.Fatalf("expected cache removal failure, got %v", err)
	}
	st, err = state.Load(app.Paths.StatePath())
	if err != nil {
		t.Fatalf("reload state: %v", err)
	}
	if len(st.Subscriptions) != 1 {
		t.Fatalf("expected subscription to remain after cache removal failure, got %+v", st.Subscriptions)
	}
	if len(st.Nodes) != 1 || st.Nodes[0].Source.Kind != "subscription" {
		t.Fatalf("expected subscription node to remain after cache removal failure, got %+v", st.Nodes)
	}
}

func TestAddRuleRejectsAUTOWithoutEnabledManualNodes(t *testing.T) {
	app, _ := newTestApp(t)
	err := app.AddRule(false, "domain", "example.com", "AUTO")
	if err == nil || !strings.Contains(err.Error(), "AUTO 需要至少一个启用的手动节点") {
		t.Fatalf("expected AUTO target guard, got %v", err)
	}
}

func TestAddRuleRejectsSubscriptionNodeTarget(t *testing.T) {
	app, _ := newTestApp(t)
	app.Client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return textResponse(http.StatusOK, "trojan://password@example.org:443?security=tls#sub-target\n"), nil
		}),
	}
	if err := app.AddSubscription("target-sub", "https://subscription.example.com/target.txt", true); err != nil {
		t.Fatalf("add subscription: %v", err)
	}
	if err := app.UpdateSubscriptions(); err != nil {
		t.Fatalf("update subscriptions: %v", err)
	}
	err := app.AddRule(false, "domain", "example.com", "sub-target")
	if err == nil || !strings.Contains(err.Error(), "未知规则目标: sub-target") {
		t.Fatalf("expected subscription target rejection, got %v", err)
	}
	st, err := state.Load(app.Paths.StatePath())
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if len(st.Rules) != 0 {
		t.Fatalf("expected no rule persisted for subscription target, got %+v", st.Rules)
	}
}

func TestAddRuleRejectsUnsupportedKindBeforePersisting(t *testing.T) {
	app, _ := newTestApp(t)
	err := app.AddRule(false, "custom-kind", "example.com", "DIRECT")
	if err == nil || !strings.Contains(err.Error(), "unsupported rule kind: custom-kind") {
		t.Fatalf("expected unsupported kind error, got %v", err)
	}
	st, err := state.Load(app.Paths.StatePath())
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if len(st.Rules) != 0 || len(st.ACL) != 0 {
		t.Fatalf("expected no invalid rules persisted, got rules=%+v acl=%+v", st.Rules, st.ACL)
	}
}

func TestAddRuleRejectsEmptyPatternBeforePersisting(t *testing.T) {
	app, _ := newTestApp(t)
	err := app.AddRule(false, "domain", "   ", "DIRECT")
	if err == nil || !strings.Contains(err.Error(), "rule pattern is empty") {
		t.Fatalf("expected empty pattern error, got %v", err)
	}
	st, err := state.Load(app.Paths.StatePath())
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if len(st.Rules) != 0 || len(st.ACL) != 0 {
		t.Fatalf("expected no empty-pattern rules persisted, got rules=%+v acl=%+v", st.Rules, st.ACL)
	}
}

func TestAddRuleTrimsPatternAndTargetBeforePersisting(t *testing.T) {
	app, _ := newTestApp(t)
	if err := app.AddRule(false, " domain-suffix ", " example.com ", " DIRECT "); err != nil {
		t.Fatalf("add trimmed rule: %v", err)
	}
	st, err := state.Load(app.Paths.StatePath())
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if len(st.Rules) != 1 {
		t.Fatalf("expected one rule, got %+v", st.Rules)
	}
	rule := st.Rules[0]
	if rule.Kind != "suffix" || rule.Pattern != "example.com" || rule.Target != "DIRECT" {
		t.Fatalf("expected normalized rule values, got %+v", rule)
	}
}

func TestApplyRulesSkipsTransparentRulesForExplicitProxyOnlyConfig(t *testing.T) {
	app, _ := newTestApp(t)
	cfg, err := config.Ensure(app.Paths.ConfigPath())
	if err != nil {
		t.Fatalf("ensure config: %v", err)
	}
	cfg.Network.ProxyIngressInterfaces = nil
	cfg.Network.DNSHijackEnabled = false
	cfg.Network.DNSHijackInterfaces = nil
	cfg.Network.ProxyHostOutput = false
	if err := config.Save(app.Paths.ConfigPath(), cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	var calls []commandCall
	app.Runner = fakeRunner{
		runFn: func(name string, args ...string) error {
			calls = append(calls, commandCall{name: name, args: append([]string{}, args...)})
			if name == "iptables" {
				return errors.New("missing")
			}
			if name == "ip" && len(args) >= 4 && args[0] == "-4" && args[1] == "rule" && args[2] == "del" {
				return errors.New("missing")
			}
			return nil
		},
	}
	err = app.ApplyRules()
	if os.Geteuid() != 0 {
		if err == nil || !strings.Contains(err.Error(), "请用 root 运行") {
			t.Fatalf("expected root error, got %v", err)
		}
		return
	}
	if err != nil {
		t.Fatalf("apply rules: %v", err)
	}
	if !strings.Contains(app.Stdout.(*bytes.Buffer).String(), "当前模板为仅显式代理，不下发透明旁路由规则") {
		t.Fatalf("unexpected apply output:\n%s", app.Stdout.(*bytes.Buffer).String())
	}
	if hasRecordedCall(calls, "iptables", "-A", "MIHOMO_PRE_HANDLE", "-p", "tcp", "-j", "TPROXY") {
		t.Fatalf("did not expect transparent routing rules in explicit-proxy-only mode: %#v", calls)
	}
}

func TestApplyRulesPropagatesExplicitProxyOnlyClearRulesFailure(t *testing.T) {
	app, _ := newTestApp(t)
	oldGeteuid := geteuid
	geteuid = func() int { return 0 }
	defer func() { geteuid = oldGeteuid }()

	cfg, err := config.Ensure(app.Paths.ConfigPath())
	if err != nil {
		t.Fatalf("ensure config: %v", err)
	}
	cfg.Network.ProxyIngressInterfaces = nil
	cfg.Network.DNSHijackEnabled = false
	cfg.Network.DNSHijackInterfaces = nil
	cfg.Network.ProxyHostOutput = false
	if err := config.Save(app.Paths.ConfigPath(), cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	var calls []commandCall
	app.Runner = fakeRunner{
		runFn: func(name string, args ...string) error {
			calls = append(calls, commandCall{name: name, args: append([]string{}, args...)})
			if name == "systemctl" && len(args) >= 2 && (args[0] == "is-active" || args[0] == "is-enabled") {
				return errors.New("inactive")
			}
			if name == "iptables" && hasArgSequence(args, "-C", "PREROUTING", "-j", "MIHOMO_PRE") {
				return nil
			}
			if name == "iptables" && hasArgSequence(args, "-D", "PREROUTING", "-j", "MIHOMO_PRE") {
				return errors.New("clear jump failed")
			}
			return errors.New("missing")
		},
	}

	err = app.ApplyRules()
	if err == nil || !strings.Contains(err.Error(), "clear jump failed") {
		t.Fatalf("expected clear-rules failure, got %v, calls=%#v", err, calls)
	}
	if strings.Contains(app.Stdout.(*bytes.Buffer).String(), "当前模板为仅显式代理") {
		t.Fatalf("did not expect skip success output after clear failure:\n%s", app.Stdout.(*bytes.Buffer).String())
	}
}

func TestApplyRulesExplicitProxyOnlyClearsExistingRulesWithoutManualNodes(t *testing.T) {
	app, _ := newTestApp(t)
	oldGeteuid := geteuid
	geteuid = func() int { return 0 }
	defer func() { geteuid = oldGeteuid }()

	cfg, err := config.Ensure(app.Paths.ConfigPath())
	if err != nil {
		t.Fatalf("ensure config: %v", err)
	}
	cfg.Network.ProxyIngressInterfaces = nil
	cfg.Network.DNSHijackEnabled = false
	cfg.Network.DNSHijackInterfaces = nil
	cfg.Network.ProxyHostOutput = false
	if err := config.Save(app.Paths.ConfigPath(), cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	var calls []commandCall
	checkHits := 0
	app.Runner = fakeRunner{
		runFn: func(name string, args ...string) error {
			calls = append(calls, commandCall{name: name, args: append([]string{}, args...)})
			if name == "systemctl" && len(args) >= 2 && (args[0] == "is-active" || args[0] == "is-enabled") {
				return errors.New("inactive")
			}
			if name == "iptables" && hasArgSequence(args, "-C", "PREROUTING", "-j", "MIHOMO_PRE") {
				if checkHits == 0 {
					checkHits++
					return nil
				}
				return errors.New("missing")
			}
			if name == "iptables" && hasArgSequence(args, "-D", "PREROUTING", "-j", "MIHOMO_PRE") {
				return nil
			}
			if name == "ip" && len(args) >= 4 && args[0] == "-4" && args[1] == "rule" && args[2] == "del" {
				return errors.New("missing")
			}
			return errors.New("missing")
		},
	}

	if err := app.ApplyRules(); err != nil {
		t.Fatalf("apply explicit-proxy-only rules: %v", err)
	}
	if !hasRecordedCall(calls, "iptables", "-w", "5", "-t", "mangle", "-D", "PREROUTING", "-j", "MIHOMO_PRE") {
		t.Fatalf("expected existing transparent jump to be removed, calls=%#v", calls)
	}
	if hasRecordedCall(calls, "iptables", "-w", "5", "-t", "mangle", "-A", "MIHOMO_PRE_HANDLE", "-p", "tcp", "-j", "TPROXY") {
		t.Fatalf("did not expect transparent TPROXY programming, calls=%#v", calls)
	}
	if !strings.Contains(app.Stdout.(*bytes.Buffer).String(), "当前模板为仅显式代理，不下发透明旁路由规则") {
		t.Fatalf("unexpected apply output:\n%s", app.Stdout.(*bytes.Buffer).String())
	}
}

func TestApplyRulesPropagatesEnsureChainFailure(t *testing.T) {
	app, _ := newTestApp(t)
	oldGeteuid := geteuid
	geteuid = func() int { return 0 }
	defer func() { geteuid = oldGeteuid }()

	app.Stdin = strings.NewReader("trojan://password@example.org:443?security=tls#apply-node\n")
	if err := app.ImportLinks(); err != nil {
		t.Fatalf("import links: %v", err)
	}
	if err := app.SetNodeEnabled(1, true); err != nil {
		t.Fatalf("enable node: %v", err)
	}
	app.Runner = fakeRunner{
		runFn: func(name string, args ...string) error {
			if name == "iptables" && hasArgSequence(args, "-C") {
				return errors.New("missing")
			}
			if name == "iptables" && hasArgSequence(args, "-S", "MIHOMO_PRE") {
				return errors.New("missing")
			}
			if name == "iptables" && hasArgSequence(args, "-N", "MIHOMO_PRE") {
				return errors.New("create failed")
			}
			if name == "ip" && len(args) >= 4 && args[0] == "-4" && args[1] == "rule" && args[2] == "del" {
				return errors.New("missing")
			}
			return nil
		},
	}
	if err := app.ApplyRules(); err == nil || !strings.Contains(err.Error(), "create failed") {
		t.Fatalf("expected ensure-chain failure, got %v", err)
	}
}

func TestApplyRulesPropagatesIptablesAppendFailure(t *testing.T) {
	app, _ := newTestApp(t)
	oldGeteuid := geteuid
	geteuid = func() int { return 0 }
	defer func() { geteuid = oldGeteuid }()

	app.Stdin = strings.NewReader("trojan://password@example.org:443?security=tls#apply-node\n")
	if err := app.ImportLinks(); err != nil {
		t.Fatalf("import links: %v", err)
	}
	if err := app.SetNodeEnabled(1, true); err != nil {
		t.Fatalf("enable node: %v", err)
	}
	app.Runner = fakeRunner{
		runFn: func(name string, args ...string) error {
			if name == "iptables" && hasArgSequence(args, "-A", "MIHOMO_PRE_HANDLE", "-p", "tcp", "-j", "TPROXY") {
				return errors.New("tproxy append failed")
			}
			if name == "iptables" {
				for _, arg := range args {
					if arg == "-C" || arg == "-S" {
						return errors.New("missing")
					}
				}
			}
			if name == "ip" && len(args) >= 4 && args[0] == "-4" && args[1] == "rule" && args[2] == "del" {
				return errors.New("missing")
			}
			return nil
		},
	}
	if err := app.ApplyRules(); err == nil || !strings.Contains(err.Error(), "tproxy append failed") {
		t.Fatalf("expected tproxy append failure, got %v", err)
	}
	if strings.Contains(app.Stdout.(*bytes.Buffer).String(), "已应用路由规则") {
		t.Fatalf("did not expect success output after append failure:\n%s", app.Stdout.(*bytes.Buffer).String())
	}
}

func TestApplyRulesPropagatesRouteProgrammingFailures(t *testing.T) {
	oldGeteuid := geteuid
	geteuid = func() int { return 0 }
	defer func() { geteuid = oldGeteuid }()

	for _, tc := range []struct {
		name string
		fail func(args []string) bool
		want string
	}{
		{
			name: "route-replace",
			fail: func(args []string) bool {
				return hasArgSequence(args, "-4", "route", "replace", "local")
			},
			want: "route replace failed",
		},
		{
			name: "rule-add",
			fail: func(args []string) bool {
				return hasArgSequence(args, "-4", "rule", "add", "fwmark", "9011")
			},
			want: "rule add failed",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app, _ := newTestApp(t)
			app.Stdin = strings.NewReader("trojan://password@example.org:443?security=tls#apply-node\n")
			if err := app.ImportLinks(); err != nil {
				t.Fatalf("import links: %v", err)
			}
			if err := app.SetNodeEnabled(1, true); err != nil {
				t.Fatalf("enable node: %v", err)
			}
			app.Runner = fakeRunner{
				runFn: func(name string, args ...string) error {
					if name == "iptables" {
						for _, arg := range args {
							if arg == "-C" || arg == "-S" {
								return errors.New("missing")
							}
						}
					}
					if name == "ip" && len(args) >= 4 && args[0] == "-4" && args[1] == "rule" && args[2] == "del" {
						return errors.New("missing")
					}
					if name == "ip" && tc.fail(args) {
						return errors.New(tc.want)
					}
					return nil
				},
			}
			if err := app.ApplyRules(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %q, got %v", tc.want, err)
			}
			if strings.Contains(app.Stdout.(*bytes.Buffer).String(), "已应用路由规则") {
				t.Fatalf("did not expect success output after route failure:\n%s", app.Stdout.(*bytes.Buffer).String())
			}
		})
	}
}

func TestApplyRulesPropagatesClearRulesFailureBeforeProgramming(t *testing.T) {
	app := newTestAppWithEnabledManualNode(t)
	oldGeteuid := geteuid
	geteuid = func() int { return 0 }
	defer func() { geteuid = oldGeteuid }()

	var calls []commandCall
	app.Runner = fakeRunner{
		runFn: func(name string, args ...string) error {
			calls = append(calls, commandCall{name: name, args: append([]string{}, args...)})
			if name == "systemctl" && len(args) >= 2 && (args[0] == "is-active" || args[0] == "is-enabled") {
				return errors.New("inactive")
			}
			if name == "iptables" && hasArgSequence(args, "-C", "PREROUTING", "-j", "MIHOMO_PRE") {
				return nil
			}
			if name == "iptables" && hasArgSequence(args, "-D", "PREROUTING", "-j", "MIHOMO_PRE") {
				return errors.New("clear jump failed")
			}
			return errors.New("missing")
		},
	}

	err := app.ApplyRules()
	if err == nil || !strings.Contains(err.Error(), "clear jump failed") {
		t.Fatalf("expected clear-rules failure, got %v, calls=%#v", err, calls)
	}
	if hasRecordedCall(calls, "iptables", "-w", "5", "-t", "mangle", "-N", "MIHOMO_PRE") {
		t.Fatalf("did not expect rule programming after clear failure, calls=%#v", calls)
	}
	if strings.Contains(app.Stdout.(*bytes.Buffer).String(), "已应用路由规则") {
		t.Fatalf("did not expect success output after clear failure:\n%s", app.Stdout.(*bytes.Buffer).String())
	}
}

func TestApplyRulesReturnsRootErrorWhenNotRoot(t *testing.T) {
	app, _ := newTestApp(t)
	oldGeteuid := geteuid
	geteuid = func() int { return 1000 }
	defer func() { geteuid = oldGeteuid }()

	err := app.ApplyRules()
	if err == nil || !strings.Contains(err.Error(), "请用 root 运行") {
		t.Fatalf("expected root error, got %v", err)
	}
}

func TestApplyRulesRejectsWhenNoEnabledManualNodes(t *testing.T) {
	app, _ := newTestApp(t)
	err := app.ApplyRules()
	if os.Geteuid() != 0 {
		if err == nil || !strings.Contains(err.Error(), "请用 root 运行") {
			t.Fatalf("expected root error, got %v", err)
		}
		return
	}
	if err == nil || !strings.Contains(err.Error(), "没有启用的手动节点") {
		t.Fatalf("expected missing manual node error, got %v", err)
	}
}

func TestControllerRuntimeSummaryFallsBackToLoopbackAndUsesSecret(t *testing.T) {
	app, _ := newTestApp(t)
	cfg, err := config.Ensure(app.Paths.ConfigPath())
	if err != nil {
		t.Fatalf("ensure config: %v", err)
	}
	cfg.Controller.BindAddress = "0.0.0.0"
	cfg.Controller.Secret = "runtime-secret"
	if err := config.Save(app.Paths.ConfigPath(), cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	app.Client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.Host != "127.0.0.1:19090" {
				t.Fatalf("unexpected host: %s", req.URL.Host)
			}
			if req.URL.Path != "/version" {
				t.Fatalf("unexpected path: %s", req.URL.Path)
			}
			if got := req.Header.Get("Authorization"); got != "Bearer runtime-secret" {
				t.Fatalf("unexpected auth header: %s", got)
			}
			return textResponse(http.StatusOK, "Mihomo Meta v9.9.9\n"), nil
		}),
	}
	summary, err := app.controllerRuntimeSummary(cfg)
	if err != nil {
		t.Fatalf("controller runtime summary: %v", err)
	}
	if summary != "Mihomo Meta v9.9.9" {
		t.Fatalf("unexpected summary: %q", summary)
	}
}

func TestControllerRuntimeSummaryReturnsBodyReadError(t *testing.T) {
	app, _ := newTestApp(t)
	cfg := config.Default()
	app.Client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       errorReadCloser{err: errors.New("version body read failed")},
				Header:     make(http.Header),
			}, nil
		}),
	}
	if _, err := app.controllerRuntimeSummary(cfg); err == nil || !strings.Contains(err.Error(), "version body read failed") {
		t.Fatalf("expected body read error, got %v", err)
	}
}

func TestControllerConfigModeFallsBackToLoopbackAndUsesSecret(t *testing.T) {
	app, _ := newTestApp(t)
	cfg, err := config.Ensure(app.Paths.ConfigPath())
	if err != nil {
		t.Fatalf("ensure config: %v", err)
	}
	cfg.Controller.BindAddress = "*"
	cfg.Controller.Secret = "config-secret"
	if err := config.Save(app.Paths.ConfigPath(), cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	app.Client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.Host != "127.0.0.1:19090" {
				t.Fatalf("unexpected host: %s", req.URL.Host)
			}
			if req.URL.Path != "/configs" {
				t.Fatalf("unexpected path: %s", req.URL.Path)
			}
			if got := req.Header.Get("Authorization"); got != "Bearer config-secret" {
				t.Fatalf("unexpected auth header: %s", got)
			}
			return textResponse(http.StatusOK, `{"mode":"direct"}`), nil
		}),
	}
	mode, err := app.controllerConfigMode(cfg)
	if err != nil {
		t.Fatalf("controller config mode: %v", err)
	}
	if mode != "direct" {
		t.Fatalf("unexpected mode: %q", mode)
	}
}

func TestControllerConfigModeHandlesInvalidAndMissingModeResponses(t *testing.T) {
	app, _ := newTestApp(t)
	cfg, err := config.Ensure(app.Paths.ConfigPath())
	if err != nil {
		t.Fatalf("ensure config: %v", err)
	}
	app.Client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return textResponse(http.StatusOK, `{"mode":`), nil
		}),
	}
	if _, err := app.controllerConfigMode(cfg); err == nil {
		t.Fatalf("expected decode error for invalid config response")
	}

	app.Client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return textResponse(http.StatusOK, `{"mode": 7}`), nil
		}),
	}
	mode, err := app.controllerConfigMode(cfg)
	if err != nil {
		t.Fatalf("controller config mode: %v", err)
	}
	if mode != "" {
		t.Fatalf("expected empty mode for non-string payload, got %q", mode)
	}
}

func TestAppProviderHelpersCountAndDetectReadyProvidersWithState(t *testing.T) {
	app, _ := newTestApp(t)
	st := state.Empty()
	if app.hasReadyProviders(st) {
		t.Fatalf("expected no ready providers in empty state")
	}
	app.Stdin = strings.NewReader("trojan://password@example.org:443?security=tls#helper-node\n")
	if err := app.ImportLinks(); err != nil {
		t.Fatalf("import links: %v", err)
	}
	if err := app.SetNodeEnabled(1, true); err != nil {
		t.Fatalf("enable node: %v", err)
	}
	if err := app.AddSubscription("helper-sub", "https://subscription.example.com/helper.txt", true); err != nil {
		t.Fatalf("add subscription: %v", err)
	}
	st, err := state.Load(app.Paths.StatePath())
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if err := os.WriteFile(app.Paths.SubscriptionFile(st.Subscriptions[0].ID), []byte("trojan://password@example.org:443?security=tls#helper-sub-node\n"), 0o640); err != nil {
		t.Fatalf("write subscription cache: %v", err)
	}
	if got := app.manualNodeCount(st); got != 1 {
		t.Fatalf("expected one manual node, got %d", got)
	}
	if !app.hasEnabledManualNodes(st) {
		t.Fatalf("expected enabled manual node")
	}
	enabled, total, ready := app.subscriptionCounts(st)
	if enabled != 1 || total != 1 || ready != 1 {
		t.Fatalf("unexpected subscription counts: enabled=%d total=%d ready=%d", enabled, total, ready)
	}
	if !app.hasReadyProviders(st) {
		t.Fatalf("expected ready providers")
	}
}

func TestValidateRuleTargetsRejectsUnknownTargetAndAllowsBuiltins(t *testing.T) {
	app, _ := newTestApp(t)
	st := state.Empty()
	st.Nodes = []state.Node{{
		ID:         "1",
		Name:       "manual-node",
		Enabled:    true,
		URI:        "trojan://password@example.org:443?security=tls#manual-node",
		ImportedAt: state.NowISO(),
		Source:     state.Source{Kind: "manual"},
	}}
	st.Rules = []state.Rule{{ID: "rule-1", Kind: "domain", Pattern: "example.com", Target: "manual-node"}}
	st.ACL = []state.Rule{{ID: "rule-2", Kind: "src-cidr", Pattern: "192.168.2.10/32", Target: "DIRECT"}}
	if err := app.validateRuleTargets(st); err != nil {
		t.Fatalf("validate rule targets: %v", err)
	}
	st.Rules[0].Target = "ghost"
	err := app.validateRuleTargets(st)
	if err == nil || !strings.Contains(err.Error(), "无效规则目标: ghost") {
		t.Fatalf("expected invalid target error, got %v", err)
	}
}

func TestValidateTargetValueGuardsAUTOWithoutManualNodes(t *testing.T) {
	app, _ := newTestApp(t)
	st := state.Empty()
	if err := app.validateTargetValue(st, "AUTO"); err == nil || !strings.Contains(err.Error(), "AUTO 需要至少一个启用的手动节点") {
		t.Fatalf("expected AUTO guard, got %v", err)
	}
	if err := app.validateTargetValue(st, "DIRECT"); err != nil {
		t.Fatalf("expected DIRECT to be allowed: %v", err)
	}
}

func TestNodeAtAndSubscriptionAtReportRangeErrors(t *testing.T) {
	app, _ := newTestApp(t)
	st := state.Empty()
	if _, err := app.nodeAt(&st, 1); err == nil || !strings.Contains(err.Error(), "node index out of range") {
		t.Fatalf("expected node range error, got %v", err)
	}
	st.Subscriptions = []state.Subscription{{ID: "sub-1"}}
	if _, err := subscriptionAt(&st, 2); err == nil || !strings.Contains(err.Error(), "subscription index out of range") {
		t.Fatalf("expected subscription range error, got %v", err)
	}
}

func TestCurrentModePrefersRuntimeModeAndFallsBackToProfileMode(t *testing.T) {
	app, _ := newTestApp(t)
	cfg := config.Default()
	mode, source := app.currentMode(cfg)
	if mode != "rule" || source != "config" {
		t.Fatalf("expected config mode fallback, got mode=%q source=%q", mode, source)
	}
	app.Client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return textResponse(http.StatusOK, `{"mode":"global"}`), nil
		}),
	}
	mode, source = app.currentMode(cfg)
	if mode != "global" || source != "runtime" {
		t.Fatalf("expected runtime mode, got mode=%q source=%q", mode, source)
	}
}

func TestNormalizeRuleHelpersMapLegacyKinds(t *testing.T) {
	for _, tc := range []struct {
		kind string
		want string
	}{
		{"domain", "domain"},
		{"suffix", "suffix"},
		{"domain-suffix", "suffix"},
		{"domain_keyword", "keyword"},
		{"src", "src-cidr"},
		{"dst", "ip-cidr"},
		{"rule-set", "ruleset"},
	} {
		if got := normalizeRuleInput(tc.kind); got != tc.want {
			t.Fatalf("kind=%q expected %q, got %q", tc.kind, tc.want, got)
		}
	}
	for _, tc := range []struct {
		kind string
		want string
	}{
		{"domain", "DOMAIN"},
		{"suffix", "DOMAIN-SUFFIX"},
		{"keyword", "DOMAIN-KEYWORD"},
		{"src-cidr", "SRC-IP-CIDR"},
		{"ip-cidr", "IP-CIDR"},
		{"ruleset", "RULE-SET"},
	} {
		if got := normalizeRuleKind(tc.kind); got != tc.want {
			t.Fatalf("kind=%q expected %q, got %q", tc.kind, tc.want, got)
		}
	}
}

func TestNormalizeRuleHelpersCoverExtendedKindsAndFallbacks(t *testing.T) {
	for _, tc := range []struct {
		kind string
		want string
	}{
		{"port", "port"},
		{"dst-port", "port"},
		{"geoip", "geoip"},
		{"geosite", "geosite"},
		{"  CUSTOM-KIND  ", "custom-kind"},
	} {
		if got := normalizeRuleInput(tc.kind); got != tc.want {
			t.Fatalf("normalizeRuleInput(%q) expected %q, got %q", tc.kind, tc.want, got)
		}
	}
	for _, tc := range []struct {
		kind string
		want string
	}{
		{"port", "DST-PORT"},
		{"geoip", "GEOIP"},
		{"geosite", "GEOSITE"},
		{"custom-kind", "CUSTOM-KIND"},
	} {
		if got := normalizeRuleKind(tc.kind); got != tc.want {
			t.Fatalf("normalizeRuleKind(%q) expected %q, got %q", tc.kind, tc.want, got)
		}
	}
}

func TestSplitFieldsAndBoolIntHelpers(t *testing.T) {
	if got := splitFields("  a  b   c "); len(got) != 3 || got[0] != "a" || got[2] != "c" {
		t.Fatalf("unexpected split fields: %#v", got)
	}
	if splitFields("   ") != nil {
		t.Fatalf("expected blank split to return nil")
	}
	if boolInt(true) != 1 || boolInt(false) != 0 {
		t.Fatalf("unexpected boolInt output")
	}
}

func TestPromptHelpersKeepDefaultsAndAcceptExplicitValues(t *testing.T) {
	var out bytes.Buffer
	if got := promptList(bufio.NewReader(strings.NewReader("\n")), &out, "LAN 接口", []string{"lan0"}); len(got) != 1 || got[0] != "lan0" {
		t.Fatalf("expected promptList to keep defaults, got %#v", got)
	}
	if got := promptList(bufio.NewReader(strings.NewReader("lan1 lan2\n")), &out, "LAN 接口", []string{"lan0"}); len(got) != 2 || got[1] != "lan2" {
		t.Fatalf("expected promptList to parse explicit values, got %#v", got)
	}
	if got := promptBool(bufio.NewReader(strings.NewReader("\n")), &out, "宿主机流量接管", true); !got {
		t.Fatalf("expected promptBool blank input to keep default")
	}
	if got := promptBool(bufio.NewReader(strings.NewReader("1\n")), &out, "宿主机流量接管", false); !got {
		t.Fatalf("expected promptBool to accept explicit true")
	}
	if got := promptBool(bufio.NewReader(strings.NewReader("0\n")), &out, "宿主机流量接管", true); got {
		t.Fatalf("expected promptBool to accept explicit false")
	}
}

func TestMenuShowsInvalidSelectionThenExit(t *testing.T) {
	app, _ := newTestApp(t)
	app.Stdin = strings.NewReader("x\n0\n")
	if err := app.Menu(); err != nil {
		t.Fatalf("menu: %v", err)
	}
	output := app.Stdout.(*bytes.Buffer).String()
	if !strings.Contains(output, "无效选择") {
		t.Fatalf("expected invalid selection output:\n%s", output)
	}
	if strings.Count(output, "0) 退出") < 2 {
		t.Fatalf("expected menu to render twice:\n%s", output)
	}
}

func TestMenuReportsActionErrorsToStderr(t *testing.T) {
	app, _ := newTestApp(t)
	oldGeteuid := geteuid
	geteuid = func() int { return 0 }
	defer func() { geteuid = oldGeteuid }()

	app.Runner = fakeRunner{
		runFn: func(name string, args ...string) error {
			if name == "systemctl" && len(args) >= 2 && args[0] == "enable" {
				return errors.New("enable failed")
			}
			return nil
		},
	}
	app.Stdin = strings.NewReader("trojan://password@example.org:443?security=tls#menu-error-node\n")
	if err := app.ImportLinks(); err != nil {
		t.Fatalf("import links: %v", err)
	}
	if err := app.SetNodeEnabled(1, true); err != nil {
		t.Fatalf("enable node: %v", err)
	}
	app.Stdin = strings.NewReader("2\n1\n0\n")
	if err := app.Menu(); err != nil {
		t.Fatalf("menu: %v", err)
	}
	if !strings.Contains(app.Stderr.(*bytes.Buffer).String(), "enable failed") {
		t.Fatalf("expected menu to report action error, stderr=%q", app.Stderr.(*bytes.Buffer).String())
	}
}

func TestMenuDispatchesSubscriptionUpdate(t *testing.T) {
	app, _ := newTestApp(t)
	app.Client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return textResponse(http.StatusOK, "trojan://password@example.org:443?security=tls#menu-sub-node\n"), nil
		}),
	}
	if err := app.AddSubscription("menu-sub", "https://subscription.example.com/menu.txt", true); err != nil {
		t.Fatalf("add subscription: %v", err)
	}
	app.Stdin = strings.NewReader("4\n6\n0\n")
	if err := app.Menu(); err != nil {
		t.Fatalf("menu: %v", err)
	}
	body, err := os.ReadFile(app.Paths.StatePath())
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	text := string(body)
	for _, needle := range []string{`"name": "menu-sub"`, `"last_count": 1`, `"name": "menu-sub-node"`} {
		if !strings.Contains(text, needle) {
			t.Fatalf("missing %q in state:\n%s", needle, text)
		}
	}
}

func TestRouterWizardPersistsUpdatedConfig(t *testing.T) {
	app, _ := newTestApp(t)
	app.Stdin = strings.NewReader(strings.Join([]string{
		"lan0 lan1",
		"lan0",
		"lan0",
		"1",
		"10.0.0.0/24 10.0.1.0/24",
		"10.0.99.0/24",
		"user:pass",
		"192.168.0.",
		"https://a.example https://b.example",
		"1",
		"qbittorrent prowlarr",
		"172.18.0.0/16",
		"8.8.8.8/32",
		"1000 1001",
	}, "\n") + "\n")
	if err := app.RouterWizard(); err != nil {
		t.Fatalf("router wizard: %v", err)
	}
	cfgBody, err := os.ReadFile(app.Paths.ConfigPath())
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	cfgText := string(cfgBody)
	for _, needle := range []string{
		"lan_interfaces:",
		"- lan0",
		"- lan1",
		"proxy_host_output: true",
		"- 10.0.99.0/24",
		"- user:pass",
		"- https://a.example",
		"- qbittorrent",
		`- "1000"`,
	} {
		if !strings.Contains(cfgText, needle) {
			t.Fatalf("missing %q in config:\n%s", needle, cfgText)
		}
	}
	output := app.Stdout.(*bytes.Buffer).String()
	if !strings.Contains(output, "旁路由参数已更新") {
		t.Fatalf("unexpected router wizard output:\n%s", output)
	}
}

func TestRenderConfigIncludesSubscriptionProviderAfterUpdate(t *testing.T) {
	app, _ := newTestApp(t)
	app.Client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return textResponse(http.StatusOK, "trojan://password@example.org:443?security=tls#sub-node\n"), nil
		}),
	}
	if err := app.AddSubscription("demo-sub", "https://subscription.example.com/sub.txt", true); err != nil {
		t.Fatalf("add subscription: %v", err)
	}
	if err := app.UpdateSubscriptions(); err != nil {
		t.Fatalf("update subscriptions: %v", err)
	}
	if err := app.RenderConfig(); err != nil {
		t.Fatalf("render config: %v", err)
	}
	stateBody, err := os.ReadFile(app.Paths.StatePath())
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	stateText := string(stateBody)
	if strings.Count(stateText, `"kind": "subscription"`) != 1 {
		t.Fatalf("unexpected subscription node count in state:\n%s", stateText)
	}
	configBody, err := os.ReadFile(app.Paths.RuntimeConfig())
	if err != nil {
		t.Fatalf("read runtime config: %v", err)
	}
	configText := string(configBody)
	for _, needle := range []string{
		"proxy-providers:",
		"subscription-",
		"path: ./proxy_providers/subscriptions/",
		`- name: "AUTO"`,
		`MATCH,PROXY`,
	} {
		if !strings.Contains(configText, needle) {
			t.Fatalf("missing %q in runtime config:\n%s", needle, configText)
		}
	}
}

func TestRenderConfigIncludesCustomRuleTargetsAndProviderMix(t *testing.T) {
	app, _ := newTestApp(t)
	app.Client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return textResponse(http.StatusOK, "trojan://password@example.org:443?security=tls#sub-mix-node\n"), nil
		}),
	}
	app.Stdin = strings.NewReader("trojan://password@example.org:443?security=tls#manual-mix-node\n")
	if err := app.ImportLinks(); err != nil {
		t.Fatalf("import links: %v", err)
	}
	if err := app.SetNodeEnabled(1, true); err != nil {
		t.Fatalf("enable node: %v", err)
	}
	if err := app.AddSubscription("mix-sub", "https://subscription.example.com/mix.txt", true); err != nil {
		t.Fatalf("add subscription: %v", err)
	}
	if err := app.UpdateSubscriptions(); err != nil {
		t.Fatalf("update subscriptions: %v", err)
	}
	if err := app.AddRule(false, "domain", "example.com", "AUTO"); err != nil {
		t.Fatalf("add rule: %v", err)
	}
	if err := app.AddRule(true, "src-cidr", "192.168.2.10/32", "manual-mix-node"); err != nil {
		t.Fatalf("add acl: %v", err)
	}
	if err := app.RenderConfig(); err != nil {
		t.Fatalf("render config: %v", err)
	}
	configBody, err := os.ReadFile(app.Paths.RuntimeConfig())
	if err != nil {
		t.Fatalf("read runtime config: %v", err)
	}
	configText := string(configBody)
	for _, needle := range []string{
		"manual:",
		"subscription-",
		"DOMAIN,example.com,AUTO",
		"SRC-IP-CIDR,192.168.2.10/32,manual-mix-node",
		`- name: "PROXY"`,
		`- name: "AUTO"`,
	} {
		if !strings.Contains(configText, needle) {
			t.Fatalf("missing %q in runtime config:\n%s", needle, configText)
		}
	}
}

func TestRenderConfigWithoutProvidersUsesDirectOnlyProxyGroup(t *testing.T) {
	app, _ := newTestApp(t)
	if err := app.RenderConfig(); err != nil {
		t.Fatalf("render config: %v", err)
	}
	configBody, err := os.ReadFile(app.Paths.RuntimeConfig())
	if err != nil {
		t.Fatalf("read runtime config: %v", err)
	}
	configText := string(configBody)
	if strings.Contains(configText, "proxy-providers:") {
		t.Fatalf("did not expect proxy-providers section:\n%s", configText)
	}
	if !strings.Contains(configText, `- name: "PROXY"`) || !strings.Contains(configText, "      - DIRECT") {
		t.Fatalf("expected direct-only proxy group:\n%s", configText)
	}
	if strings.Contains(configText, `- name: "AUTO"`) {
		t.Fatalf("did not expect AUTO group without providers:\n%s", configText)
	}
}

func TestRenderConfigIncludesAuthenticationAndCORSSections(t *testing.T) {
	app, _ := newTestApp(t)
	cfg, err := config.Ensure(app.Paths.ConfigPath())
	if err != nil {
		t.Fatalf("ensure config: %v", err)
	}
	cfg.Access.Authentication = []string{"user:pass"}
	cfg.Access.SkipAuthPrefixes = []string{"192.168.2."}
	cfg.Controller.CORSAllowOrigins = []string{"https://panel.example"}
	cfg.Controller.CORSAllowPrivateNetwork = true
	if err := config.Save(app.Paths.ConfigPath(), cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	if err := app.RenderConfig(); err != nil {
		t.Fatalf("render config: %v", err)
	}
	configBody, err := os.ReadFile(app.Paths.RuntimeConfig())
	if err != nil {
		t.Fatalf("read runtime config: %v", err)
	}
	configText := string(configBody)
	for _, needle := range []string{
		"authentication:",
		`  - "user:pass"`,
		"skip-auth-prefixes:",
		"  - 192.168.2.",
		"external-controller-cors:",
		`    - "https://panel.example"`,
		"  allow-private-network: true",
	} {
		if !strings.Contains(configText, needle) {
			t.Fatalf("missing %q in runtime config:\n%s", needle, configText)
		}
	}
}

func TestRenderConfigSupportsExplicitProxyOnlyConfig(t *testing.T) {
	app, _ := newTestApp(t)
	cfg, err := config.Ensure(app.Paths.ConfigPath())
	if err != nil {
		t.Fatalf("ensure config: %v", err)
	}
	cfg.Network.ProxyIngressInterfaces = nil
	cfg.Network.DNSHijackEnabled = false
	cfg.Network.DNSHijackInterfaces = nil
	cfg.Network.ProxyHostOutput = false
	cfg.Network.LANCIDRs = []string{"10.10.0.0/24"}
	if err := config.Save(app.Paths.ConfigPath(), cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	if err := app.RenderConfig(); err != nil {
		t.Fatalf("render config: %v", err)
	}
	configBody, err := os.ReadFile(app.Paths.RuntimeConfig())
	if err != nil {
		t.Fatalf("read runtime config: %v", err)
	}
	configText := string(configBody)
	if !strings.Contains(configText, "lan-allowed-ips:\n  - 10.10.0.0/24\n  - 127.0.0.0/8") {
		t.Fatalf("expected updated LAN CIDRs:\n%s", configText)
	}
	if strings.Contains(configText, "proxy-providers:") {
		t.Fatalf("did not expect providers for explicit-proxy-only config:\n%s", configText)
	}
}

func TestRenderConfigIncludesBindAddressAndLANDisallowed(t *testing.T) {
	app, _ := newTestApp(t)
	cfg, err := config.Ensure(app.Paths.ConfigPath())
	if err != nil {
		t.Fatalf("ensure config: %v", err)
	}
	cfg.Controller.BindAddress = "0.0.0.0"
	cfg.Access.LANDisallowedCIDRs = []string{"10.10.10.0/24", "172.16.0.0/16"}
	if err := config.Save(app.Paths.ConfigPath(), cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	if err := app.RenderConfig(); err != nil {
		t.Fatalf("render config: %v", err)
	}
	configBody, err := os.ReadFile(app.Paths.RuntimeConfig())
	if err != nil {
		t.Fatalf("read runtime config: %v", err)
	}
	configText := string(configBody)
	for _, needle := range []string{
		"external-controller: 0.0.0.0:19090",
		"lan-disallowed-ips:",
		"  - 10.10.10.0/24",
		"  - 172.16.0.0/16",
	} {
		if !strings.Contains(configText, needle) {
			t.Fatalf("missing %q in runtime config:\n%s", needle, configText)
		}
	}
}

func TestClearRulesRunsExpectedCleanupCommands(t *testing.T) {
	app, _ := newTestApp(t)
	var calls []commandCall
	checkHits := map[string]int{}
	app.Runner = fakeRunner{
		runFn: func(name string, args ...string) error {
			calls = append(calls, commandCall{name: name, args: append([]string{}, args...)})
			if name == "iptables" {
				key := strings.Join(args, " ")
				if strings.Contains(key, "-C PREROUTING -j MIHOMO_PRE") {
					if checkHits[key] == 0 {
						checkHits[key]++
						return nil
					}
					return errors.New("missing")
				}
				if strings.Contains(key, "-C OUTPUT -j MIHOMO_OUT") {
					if checkHits[key] == 0 {
						checkHits[key]++
						return nil
					}
					return errors.New("missing")
				}
				if strings.Contains(key, "-C PREROUTING -j MIHOMO_DNS") {
					if checkHits[key] == 0 {
						checkHits[key]++
						return nil
					}
					return errors.New("missing")
				}
				if strings.Contains(key, "-C OUTPUT -j MIHOMO_DNS_OUT") {
					if checkHits[key] == 0 {
						checkHits[key]++
						return nil
					}
					return errors.New("missing")
				}
			}
			if name == "ip" && len(args) >= 4 && args[0] == "-4" && args[1] == "rule" && args[2] == "del" {
				return errors.New("missing")
			}
			return nil
		},
		outputFn: func(name string, args ...string) (string, string, error) {
			return "", "", nil
		},
	}
	if err := app.ClearRules(); err != nil {
		t.Fatalf("clear rules: %v", err)
	}
	for _, expect := range []struct {
		name string
		args []string
	}{
		{"iptables", []string{"-w", "5", "-t", "mangle", "-D", "PREROUTING", "-j", "MIHOMO_PRE"}},
		{"iptables", []string{"-w", "5", "-t", "mangle", "-F", "MIHOMO_PRE"}},
		{"iptables", []string{"-w", "5", "-t", "nat", "-X", "MIHOMO_DNS_OUT"}},
		{"ip", []string{"-4", "route", "flush", "table", "233"}},
	} {
		if !hasRecordedCall(calls, expect.name, expect.args...) {
			t.Fatalf("missing cleanup call %s %#v in %#v", expect.name, expect.args, calls)
		}
	}
}

func TestClearRulesReturnsRootErrorWhenNotRoot(t *testing.T) {
	app, _ := newTestApp(t)
	oldGeteuid := geteuid
	geteuid = func() int { return 1000 }
	defer func() { geteuid = oldGeteuid }()

	err := app.ClearRules()
	if err == nil || !strings.Contains(err.Error(), "请用 root 运行") {
		t.Fatalf("expected root error, got %v", err)
	}
}

func TestApplyRulesProgramsExpectedRoutingCommands(t *testing.T) {
	app, _ := newTestApp(t)
	var calls []commandCall
	app.Runner = fakeRunner{
		runFn: func(name string, args ...string) error {
			calls = append(calls, commandCall{name: name, args: append([]string{}, args...)})
			if name == "systemctl" && len(args) >= 2 && args[0] == "is-active" {
				return errors.New("inactive")
			}
			if name == "systemctl" && len(args) >= 2 && args[0] == "is-enabled" {
				return errors.New("disabled")
			}
			if name == "docker" {
				return errors.New("missing")
			}
			if name == "iptables" {
				for _, arg := range args {
					if arg == "-C" || arg == "-S" {
						return errors.New("missing")
					}
				}
			}
			if name == "ip" && len(args) >= 4 && args[0] == "-4" && args[1] == "rule" && args[2] == "del" {
				return errors.New("missing")
			}
			return nil
		},
		outputFn: func(name string, args ...string) (string, string, error) {
			return "", "", nil
		},
	}
	app.Stdin = strings.NewReader("trojan://password@example.org:443?security=tls#apply-node\n")
	if err := app.ImportLinks(); err != nil {
		t.Fatalf("import links: %v", err)
	}
	if err := app.SetNodeEnabled(1, true); err != nil {
		t.Fatalf("enable node: %v", err)
	}
	if err := app.ApplyRules(); err != nil {
		t.Fatalf("apply rules: %v", err)
	}
	for _, expect := range []struct {
		name string
		args []string
	}{
		{"iptables", []string{"-w", "5", "-t", "mangle", "-N", "MIHOMO_PRE"}},
		{"iptables", []string{"-w", "5", "-t", "mangle", "-A", "MIHOMO_PRE_HANDLE", "-p", "tcp", "-j", "TPROXY"}},
		{"iptables", []string{"-w", "5", "-t", "mangle", "-A", "MIHOMO_OUT", "-p", "tcp", "-j", "MARK", "--set-mark", "9011"}},
		{"iptables", []string{"-w", "5", "-t", "nat", "-A", "PREROUTING", "-j", "MIHOMO_DNS"}},
		{"ip", []string{"-4", "route", "replace", "local", "0.0.0.0/0", "dev", "lo", "table", "233"}},
		{"ip", []string{"-4", "rule", "add", "fwmark", "9011", "table", "233", "priority", "100"}},
	} {
		if !hasRecordedCall(calls, expect.name, expect.args...) {
			t.Fatalf("missing apply call %s %#v in %#v", expect.name, expect.args, calls)
		}
	}
	if !strings.Contains(app.Stdout.(*bytes.Buffer).String(), "已应用路由规则") {
		t.Fatalf("unexpected apply output:\n%s", app.Stdout.(*bytes.Buffer).String())
	}
}

func TestApplyRulesFlushesExistingChainsBeforeProgramming(t *testing.T) {
	app := newTestAppWithEnabledManualNode(t)
	oldGeteuid := geteuid
	geteuid = func() int { return 0 }
	defer func() { geteuid = oldGeteuid }()

	var calls []commandCall
	app.Runner = fakeRunner{
		runFn: func(name string, args ...string) error {
			calls = append(calls, commandCall{name: name, args: append([]string{}, args...)})
			if name == "systemctl" && len(args) >= 2 && (args[0] == "is-active" || args[0] == "is-enabled") {
				return errors.New("inactive")
			}
			if name == "iptables" && hasArgSequence(args, "-C") {
				return errors.New("missing")
			}
			if name == "iptables" && hasArgSequence(args, "-S") {
				return nil
			}
			if name == "ip" && len(args) >= 4 && args[0] == "-4" && args[1] == "rule" && args[2] == "del" {
				return errors.New("missing")
			}
			return nil
		},
		outputFn: func(name string, args ...string) (string, string, error) {
			return "", "", nil
		},
	}

	if err := app.ApplyRules(); err != nil {
		t.Fatalf("apply rules: %v", err)
	}
	for _, chain := range []string{"MIHOMO_PRE", "MIHOMO_PRE_HANDLE", "MIHOMO_OUT", "MIHOMO_DNS", "MIHOMO_DNS_HANDLE"} {
		if !hasRecordedCall(calls, "iptables", "-w", "5", "-S", chain) {
			t.Fatalf("expected chain existence check for %s, calls=%#v", chain, calls)
		}
		if !hasRecordedCall(calls, "iptables", "-w", "5", "-F", chain) {
			t.Fatalf("expected existing chain flush for %s, calls=%#v", chain, calls)
		}
		if hasRecordedCall(calls, "iptables", "-w", "5", "-N", chain) {
			t.Fatalf("did not expect create for existing chain %s, calls=%#v", chain, calls)
		}
	}
}

func TestApplyRulesSkipsExistingTopLevelJumps(t *testing.T) {
	app := newTestAppWithEnabledManualNode(t)
	oldGeteuid := geteuid
	geteuid = func() int { return 0 }
	defer func() { geteuid = oldGeteuid }()

	cfg, err := config.Ensure(app.Paths.ConfigPath())
	if err != nil {
		t.Fatalf("ensure config: %v", err)
	}
	cfg.Network.ProxyHostOutput = true
	if err := config.Save(app.Paths.ConfigPath(), cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	var calls []commandCall
	programmedRules := false
	app.Runner = fakeRunner{
		runFn: func(name string, args ...string) error {
			calls = append(calls, commandCall{name: name, args: append([]string{}, args...)})
			if name == "systemctl" && len(args) >= 2 && (args[0] == "is-active" || args[0] == "is-enabled") {
				return errors.New("inactive")
			}
			if name == "iptables" && hasArgSequence(args, "-A", "MIHOMO_PRE_HANDLE", "-p", "tcp", "-j", "TPROXY") {
				programmedRules = true
			}
			if name == "iptables" && hasArgSequence(args, "-C") {
				if programmedRules &&
					(hasArgSequence(args, "-C", "PREROUTING", "-j", "MIHOMO_DNS") ||
						hasArgSequence(args, "-C", "PREROUTING", "-j", "MIHOMO_PRE") ||
						hasArgSequence(args, "-C", "OUTPUT", "-j", "MIHOMO_OUT")) {
					return nil
				}
				return errors.New("missing")
			}
			if name == "iptables" && hasArgSequence(args, "-S") {
				return errors.New("missing")
			}
			if name == "ip" && len(args) >= 4 && args[0] == "-4" && args[1] == "rule" && args[2] == "del" {
				return errors.New("missing")
			}
			return nil
		},
		outputFn: func(name string, args ...string) (string, string, error) {
			return "", "", nil
		},
	}

	if err := app.ApplyRules(); err != nil {
		t.Fatalf("apply rules: %v", err)
	}
	for _, forbidden := range []struct {
		table string
		args  []string
	}{
		{"nat", []string{"-A", "PREROUTING", "-j", "MIHOMO_DNS"}},
		{"mangle", []string{"-A", "PREROUTING", "-j", "MIHOMO_PRE"}},
		{"mangle", []string{"-A", "OUTPUT", "-j", "MIHOMO_OUT"}},
	} {
		if hasRecordedCall(calls, "iptables", append([]string{"-w", "5", "-t", forbidden.table}, forbidden.args...)...) {
			t.Fatalf("did not expect duplicate jump %s %#v in %#v", forbidden.table, forbidden.args, calls)
		}
	}
}

func TestApplyRulesSkipsDNSAndOutputJumpsWhenDisabled(t *testing.T) {
	app, _ := newTestApp(t)
	cfg, err := config.Ensure(app.Paths.ConfigPath())
	if err != nil {
		t.Fatalf("ensure config: %v", err)
	}
	cfg.Network.DNSHijackEnabled = false
	cfg.Network.DNSHijackInterfaces = nil
	cfg.Network.ProxyHostOutput = false
	if err := config.Save(app.Paths.ConfigPath(), cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	var calls []commandCall
	app.Runner = fakeRunner{
		runFn: func(name string, args ...string) error {
			calls = append(calls, commandCall{name: name, args: append([]string{}, args...)})
			if name == "docker" {
				return errors.New("missing")
			}
			if name == "iptables" {
				for _, arg := range args {
					if arg == "-C" || arg == "-S" {
						return errors.New("missing")
					}
				}
			}
			if name == "ip" && len(args) >= 4 && args[0] == "-4" && args[1] == "rule" && args[2] == "del" {
				return errors.New("missing")
			}
			return nil
		},
		outputFn: func(name string, args ...string) (string, string, error) {
			return "", "", nil
		},
	}
	app.Stdin = strings.NewReader("trojan://password@example.org:443?security=tls#apply-node\n")
	if err := app.ImportLinks(); err != nil {
		t.Fatalf("import links: %v", err)
	}
	if err := app.SetNodeEnabled(1, true); err != nil {
		t.Fatalf("enable node: %v", err)
	}
	err = app.ApplyRules()
	if os.Geteuid() != 0 {
		if err == nil || !strings.Contains(err.Error(), "请用 root 运行") {
			t.Fatalf("expected root error, got %v", err)
		}
		return
	}
	if err != nil {
		t.Fatalf("apply rules: %v", err)
	}
	for _, call := range calls {
		if call.name == "iptables" && hasArgSequence(call.args, "-A", "PREROUTING", "-j", "MIHOMO_DNS") {
			t.Fatalf("did not expect nat PREROUTING DNS jump when dns hijack disabled: %#v", calls)
		}
		if call.name == "iptables" && hasArgSequence(call.args, "-A", "OUTPUT", "-j", "MIHOMO_OUT") {
			t.Fatalf("did not expect OUTPUT jump when proxy host output disabled: %#v", calls)
		}
		if call.name == "iptables" && hasArgSequence(call.args, "-A", "MIHOMO_DNS_HANDLE", "-p", "udp", "--dport", "53") {
			t.Fatalf("did not expect DNS redirect rules when dns hijack disabled: %#v", calls)
		}
	}
	if !hasRecordedCall(calls, "iptables", "-w", "5", "-t", "mangle", "-A", "PREROUTING", "-j", "MIHOMO_PRE") {
		t.Fatalf("expected mangle PREROUTING jump, calls=%#v", calls)
	}
}

func TestDeleteJumpReturnsRunnerErrorAfterRemovalAttempt(t *testing.T) {
	app, _ := newTestApp(t)
	var calls []commandCall
	checkHits := map[string]int{}
	app.Runner = fakeRunner{
		runFn: func(name string, args ...string) error {
			calls = append(calls, commandCall{name: name, args: append([]string{}, args...)})
			if name != "iptables" {
				return nil
			}
			key := strings.Join(args, " ")
			if strings.Contains(key, "-C PREROUTING -j MIHOMO_PRE") {
				if checkHits[key] == 0 {
					checkHits[key]++
					return nil
				}
				return errors.New("missing")
			}
			if strings.Contains(key, "-D PREROUTING -j MIHOMO_PRE") {
				return errors.New("delete failed")
			}
			return nil
		},
	}
	err := app.deleteJump("mangle", "PREROUTING", "-j", "MIHOMO_PRE")
	if err == nil || !strings.Contains(err.Error(), "delete failed") {
		t.Fatalf("expected delete failure, got %v", err)
	}
	if !hasRecordedCall(calls, "iptables", "-w", "5", "-t", "mangle", "-D", "PREROUTING", "-j", "MIHOMO_PRE") {
		t.Fatalf("expected delete call, calls=%#v", calls)
	}
}

func TestApplyRulesProgramsBypassAndDNSOutputRules(t *testing.T) {
	app, _ := newTestApp(t)
	cfg, err := config.Ensure(app.Paths.ConfigPath())
	if err != nil {
		t.Fatalf("ensure config: %v", err)
	}
	cfg.Network.DNSHijackEnabled = true
	cfg.Network.DNSHijackInterfaces = []string{"bridge1"}
	cfg.Network.ProxyHostOutput = true
	cfg.Network.Bypass.DstCIDRs = []string{"8.8.8.0/24"}
	cfg.Network.Bypass.SrcCIDRs = []string{"192.168.50.0/24"}
	cfg.Network.Bypass.ContainerNames = []string{"alpha", "beta"}
	cfg.Network.Bypass.UIDs = []string{"1000", "1001"}
	if err := config.Save(app.Paths.ConfigPath(), cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	var calls []commandCall
	app.Runner = fakeRunner{
		runFn: func(name string, args ...string) error {
			calls = append(calls, commandCall{name: name, args: append([]string{}, args...)})
			if name == "docker" {
				return errors.New("missing")
			}
			if name == "iptables" {
				for _, arg := range args {
					if arg == "-C" || arg == "-S" {
						return errors.New("missing")
					}
				}
			}
			if name == "ip" && len(args) >= 4 && args[0] == "-4" && args[1] == "rule" && args[2] == "del" {
				return errors.New("missing")
			}
			return nil
		},
		outputFn: func(name string, args ...string) (string, string, error) {
			if name == "docker" && len(args) >= 2 && args[1] == "alpha" {
				return "172.18.0.2\n", "", nil
			}
			if name == "docker" && len(args) >= 2 && args[1] == "beta" {
				return "172.18.0.3\n", "", nil
			}
			return "", "", nil
		},
	}
	app.Stdin = strings.NewReader("trojan://password@example.org:443?security=tls#apply-node\n")
	if err := app.ImportLinks(); err != nil {
		t.Fatalf("import links: %v", err)
	}
	if err := app.SetNodeEnabled(1, true); err != nil {
		t.Fatalf("enable node: %v", err)
	}
	if err := app.ApplyRules(); err != nil {
		t.Fatalf("apply rules: %v", err)
	}
	for _, expect := range []struct {
		name string
		args []string
	}{
		{"iptables", []string{"-w", "5", "-t", "mangle", "-A", "MIHOMO_PRE_HANDLE", "-d", "8.8.8.0/24", "-j", "RETURN"}},
		{"iptables", []string{"-w", "5", "-t", "mangle", "-A", "MIHOMO_PRE_HANDLE", "-s", "192.168.50.0/24", "-j", "RETURN"}},
		{"iptables", []string{"-w", "5", "-t", "mangle", "-A", "MIHOMO_PRE_HANDLE", "-s", "172.18.0.2/32", "-j", "RETURN"}},
		{"iptables", []string{"-w", "5", "-t", "mangle", "-A", "MIHOMO_OUT", "-m", "owner", "--uid-owner", "1000", "-j", "RETURN"}},
		{"iptables", []string{"-w", "5", "-t", "nat", "-A", "PREROUTING", "-j", "MIHOMO_DNS"}},
		{"iptables", []string{"-w", "5", "-t", "nat", "-A", "MIHOMO_DNS_HANDLE", "-p", "udp", "--dport", "53", "-j", "REDIRECT", "--to-ports", "1053"}},
		{"ip", []string{"-4", "route", "replace", "local", "0.0.0.0/0", "dev", "lo", "table", "233"}},
	} {
		if !hasRecordedCall(calls, expect.name, expect.args...) {
			t.Fatalf("missing apply call %s %#v in %#v", expect.name, expect.args, calls)
		}
	}
	if !strings.Contains(app.Stdout.(*bytes.Buffer).String(), "已应用路由规则") {
		t.Fatalf("unexpected apply output:\n%s", app.Stdout.(*bytes.Buffer).String())
	}
}

func TestApplyRulesProgramsDNSHijackForUDPAndTCP(t *testing.T) {
	app := newTestAppWithEnabledManualNode(t)
	oldGeteuid := geteuid
	geteuid = func() int { return 0 }
	defer func() { geteuid = oldGeteuid }()

	var calls []commandCall
	app.Runner = fakeRunner{
		runFn: func(name string, args ...string) error {
			calls = append(calls, commandCall{name: name, args: append([]string{}, args...)})
			if name == "systemctl" && len(args) >= 2 && (args[0] == "is-active" || args[0] == "is-enabled") {
				return errors.New("inactive")
			}
			if name == "iptables" {
				for _, arg := range args {
					if arg == "-C" || arg == "-S" {
						return errors.New("missing")
					}
				}
			}
			if name == "ip" && len(args) >= 4 && args[0] == "-4" && args[1] == "rule" && args[2] == "del" {
				return errors.New("missing")
			}
			return nil
		},
		outputFn: func(name string, args ...string) (string, string, error) {
			return "", "", nil
		},
	}

	if err := app.ApplyRules(); err != nil {
		t.Fatalf("apply rules: %v", err)
	}
	for _, expect := range []struct {
		name string
		args []string
	}{
		{"iptables", []string{"-w", "5", "-t", "mangle", "-A", "MIHOMO_PRE_HANDLE", "-p", "udp", "--dport", "53", "-j", "RETURN"}},
		{"iptables", []string{"-w", "5", "-t", "mangle", "-A", "MIHOMO_PRE_HANDLE", "-p", "tcp", "--dport", "53", "-j", "RETURN"}},
		{"iptables", []string{"-w", "5", "-t", "nat", "-A", "MIHOMO_DNS_HANDLE", "-p", "udp", "--dport", "53", "-j", "REDIRECT", "--to-ports", "1053"}},
		{"iptables", []string{"-w", "5", "-t", "nat", "-A", "MIHOMO_DNS_HANDLE", "-p", "tcp", "--dport", "53", "-j", "REDIRECT", "--to-ports", "1053"}},
	} {
		if !hasRecordedCall(calls, expect.name, expect.args...) {
			t.Fatalf("missing DNS hijack call %s %#v in %#v", expect.name, expect.args, calls)
		}
	}
}

func TestApplyRulesPropagatesDNSRedirectFailure(t *testing.T) {
	app := newTestAppWithEnabledManualNode(t)
	oldGeteuid := geteuid
	geteuid = func() int { return 0 }
	defer func() { geteuid = oldGeteuid }()

	app.Runner = fakeRunner{
		runFn: func(name string, args ...string) error {
			if name == "systemctl" && len(args) >= 2 && (args[0] == "is-active" || args[0] == "is-enabled") {
				return errors.New("inactive")
			}
			if name == "iptables" && hasArgSequence(args, "-A", "MIHOMO_DNS_HANDLE", "-p", "udp", "--dport", "53", "-j", "REDIRECT") {
				return errors.New("dns redirect failed")
			}
			if name == "iptables" {
				for _, arg := range args {
					if arg == "-C" || arg == "-S" {
						return errors.New("missing")
					}
				}
			}
			if name == "ip" && len(args) >= 4 && args[0] == "-4" && args[1] == "rule" && args[2] == "del" {
				return errors.New("missing")
			}
			return nil
		},
	}

	err := app.ApplyRules()
	if err == nil || !strings.Contains(err.Error(), "dns redirect failed") {
		t.Fatalf("expected DNS redirect failure, got %v", err)
	}
	if strings.Contains(app.Stdout.(*bytes.Buffer).String(), "已应用路由规则") {
		t.Fatalf("did not expect success output after DNS redirect failure:\n%s", app.Stdout.(*bytes.Buffer).String())
	}
}

func TestApplyRulesPropagatesOutputJumpFailure(t *testing.T) {
	app := newTestAppWithEnabledManualNode(t)
	oldGeteuid := geteuid
	geteuid = func() int { return 0 }
	defer func() { geteuid = oldGeteuid }()

	cfg, err := config.Ensure(app.Paths.ConfigPath())
	if err != nil {
		t.Fatalf("ensure config: %v", err)
	}
	cfg.Network.DNSHijackEnabled = false
	cfg.Network.DNSHijackInterfaces = nil
	cfg.Network.ProxyHostOutput = true
	if err := config.Save(app.Paths.ConfigPath(), cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	app.Runner = fakeRunner{
		runFn: func(name string, args ...string) error {
			if name == "systemctl" && len(args) >= 2 && (args[0] == "is-active" || args[0] == "is-enabled") {
				return errors.New("inactive")
			}
			if name == "iptables" && hasArgSequence(args, "-A", "OUTPUT", "-j", "MIHOMO_OUT") {
				return errors.New("output jump failed")
			}
			if name == "iptables" {
				for _, arg := range args {
					if arg == "-C" || arg == "-S" {
						return errors.New("missing")
					}
				}
			}
			if name == "ip" && len(args) >= 4 && args[0] == "-4" && args[1] == "rule" && args[2] == "del" {
				return errors.New("missing")
			}
			return nil
		},
	}

	err = app.ApplyRules()
	if err == nil || !strings.Contains(err.Error(), "output jump failed") {
		t.Fatalf("expected OUTPUT jump failure, got %v", err)
	}
	if strings.Contains(app.Stdout.(*bytes.Buffer).String(), "已应用路由规则") {
		t.Fatalf("did not expect success output after OUTPUT jump failure:\n%s", app.Stdout.(*bytes.Buffer).String())
	}
}

func TestRenderConfigWritesRuntimeArtifacts(t *testing.T) {
	app, _ := newTestApp(t)
	app.Stdin = strings.NewReader("trojan://password@example.org:443?security=tls#demo-node\n")
	if err := app.ImportLinks(); err != nil {
		t.Fatalf("import links: %v", err)
	}
	if err := app.SetNodeEnabled(1, true); err != nil {
		t.Fatalf("enable node: %v", err)
	}
	if err := app.RenderConfig(); err != nil {
		t.Fatalf("render config: %v", err)
	}
	raw, err := os.ReadFile(app.Paths.RuntimeConfig())
	if err != nil {
		t.Fatalf("read runtime config: %v", err)
	}
	text := string(raw)
	for _, needle := range []string{
		`mixed-port: 7890`,
		`external-controller: 127.0.0.1:19090`,
		`proxy-providers:`,
		`manual:`,
		`PROCESS-NAME,mihomo,DIRECT`,
	} {
		if !strings.Contains(text, needle) {
			t.Fatalf("missing %q in runtime config:\n%s", needle, text)
		}
	}
}

func TestRenderConfigFailsWhenBuiltinRulesPathIsDirectory(t *testing.T) {
	app, _ := newTestApp(t)
	if err := os.MkdirAll(app.Paths.BuiltinRules(), 0o755); err != nil {
		t.Fatalf("mkdir blocking builtin rules path: %v", err)
	}
	err := app.RenderConfig()
	if err == nil || !strings.Contains(err.Error(), "is a directory") {
		t.Fatalf("expected builtin rules write failure, got %v", err)
	}
}

func TestRenderConfigFailsWhenRuntimeConfigPathIsDirectory(t *testing.T) {
	app := newTestAppWithEnabledManualNode(t)
	if err := os.MkdirAll(app.Paths.RuntimeConfig(), 0o755); err != nil {
		t.Fatalf("mkdir blocking runtime config path: %v", err)
	}
	err := app.RenderConfig()
	if err == nil || !strings.Contains(err.Error(), "is a directory") {
		t.Fatalf("expected runtime config write failure, got %v", err)
	}
}

func TestUpdateSubscriptionsWritesCacheAndNodes(t *testing.T) {
	app, _ := newTestApp(t)
	app.Client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			body := io.NopCloser(strings.NewReader("trojan://password@example.org:443?security=tls#sub-node\n"))
			return &http.Response{
				StatusCode: 200,
				Body:       body,
				Header:     make(http.Header),
			}, nil
		}),
	}

	if err := app.AddSubscription("demo-sub", "https://subscription.example.com/sub.txt", true); err != nil {
		t.Fatalf("add subscription: %v", err)
	}
	if err := app.UpdateSubscriptions(); err != nil {
		t.Fatalf("update subscriptions: %v", err)
	}

	stateBody, err := os.ReadFile(app.Paths.StatePath())
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	stateText := string(stateBody)
	for _, needle := range []string{`"name": "demo-sub"`, `"last_success_at":`, `"last_count": 1`, `"kind": "subscription"`} {
		if !strings.Contains(stateText, needle) {
			t.Fatalf("missing %q in updated state:\n%s", needle, stateText)
		}
	}

	matches, err := filepath.Glob(filepath.Join(app.Paths.SubscriptionDir(), "*.txt"))
	if err != nil {
		t.Fatalf("glob cache files: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected one provider cache file, got %d", len(matches))
	}
	cacheBody, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("read provider cache: %v", err)
	}
	if !strings.Contains(string(cacheBody), "trojan://password@example.org:443?security=tls#sub-node") {
		t.Fatalf("unexpected provider cache:\n%s", string(cacheBody))
	}
}

func TestUpdateSubscriptionsRecordsBodyReadFailure(t *testing.T) {
	app, _ := newTestApp(t)
	if err := app.AddSubscription("body-fail", "https://subscription.example.com/body-fail.txt", true); err != nil {
		t.Fatalf("add subscription: %v", err)
	}
	app.Client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: 200,
				Body:       errorReadCloser{err: errors.New("read failed")},
				Header:     make(http.Header),
			}, nil
		}),
	}
	if err := app.UpdateSubscriptions(); err != nil {
		t.Fatalf("update subscriptions: %v", err)
	}
	st, err := state.Load(app.Paths.StatePath())
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if !strings.Contains(st.Subscriptions[0].Cache.LastError, "read failed") {
		t.Fatalf("expected read failure recorded, got %+v", st.Subscriptions[0].Cache)
	}
	if !strings.Contains(app.Stderr.(*bytes.Buffer).String(), "body-fail") {
		t.Fatalf("expected subscription name in stderr:\n%s", app.Stderr.(*bytes.Buffer).String())
	}
}

func TestUpdateSubscriptionsRecordsInvalidURLFailure(t *testing.T) {
	app, _ := newTestApp(t)
	if err := app.AddSubscription("invalid-url", "://bad-url", true); err != nil {
		t.Fatalf("add subscription: %v", err)
	}
	if err := app.UpdateSubscriptions(); err != nil {
		t.Fatalf("update subscriptions: %v", err)
	}
	st, err := state.Load(app.Paths.StatePath())
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if st.Subscriptions[0].Cache.LastAttemptAt == "" {
		t.Fatalf("expected attempt timestamp to be recorded, got %+v", st.Subscriptions[0].Cache)
	}
	if !strings.Contains(st.Subscriptions[0].Cache.LastError, "missing protocol scheme") {
		t.Fatalf("expected invalid URL recorded, got %+v", st.Subscriptions[0].Cache)
	}
	if !strings.Contains(app.Stderr.(*bytes.Buffer).String(), "invalid-url") {
		t.Fatalf("expected subscription name in stderr:\n%s", app.Stderr.(*bytes.Buffer).String())
	}
}

func TestUpdateSubscriptionsRecordsWriteFailure(t *testing.T) {
	app, _ := newTestApp(t)
	if err := app.AddSubscription("write-fail", "https://subscription.example.com/write-fail.txt", true); err != nil {
		t.Fatalf("add subscription: %v", err)
	}
	st, err := state.Load(app.Paths.StatePath())
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if err := os.MkdirAll(app.Paths.SubscriptionFile(st.Subscriptions[0].ID), 0o755); err != nil {
		t.Fatalf("mkdir blocking subscription file path: %v", err)
	}
	app.Client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return textResponse(http.StatusOK, "trojan://password@example.org:443?security=tls#write-node\n"), nil
		}),
	}
	if err := app.UpdateSubscriptions(); err != nil {
		t.Fatalf("update subscriptions: %v", err)
	}
	st, err = state.Load(app.Paths.StatePath())
	if err != nil {
		t.Fatalf("reload state: %v", err)
	}
	if !strings.Contains(st.Subscriptions[0].Cache.LastError, "is a directory") {
		t.Fatalf("expected write failure recorded, got %+v", st.Subscriptions[0].Cache)
	}
}
