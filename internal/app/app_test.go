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
		"2) 更新订阅",
		"1\tlist-sub\thttps://subscription.example.com/list.txt\t1\t2026-04-27T10:00:00+08:00\t2\tboom",
	} {
		if !strings.Contains(output, needle) {
			t.Fatalf("missing %q in subscriptions output:\n%s", needle, output)
		}
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

func TestRulesMenuAddsRuleAndPromptStringKeepsDefaultOnBlankInput(t *testing.T) {
	app, _ := newTestApp(t)
	app.Stdin = strings.NewReader("trojan://password@example.org:443?security=tls#menu-rule-node\n")
	if err := app.ImportLinks(); err != nil {
		t.Fatalf("import links: %v", err)
	}
	if err := app.SetNodeEnabled(1, true); err != nil {
		t.Fatalf("enable node: %v", err)
	}

	reader := bufio.NewReader(strings.NewReader("2\ndomain\nmenu.example.com\nAUTO\n"))
	if err := app.rulesMenu(reader, false); err != nil {
		t.Fatalf("rules menu add: %v", err)
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

func TestAddRuleRejectsAUTOWithoutEnabledManualNodes(t *testing.T) {
	app, _ := newTestApp(t)
	err := app.AddRule(false, "domain", "example.com", "AUTO")
	if err == nil || !strings.Contains(err.Error(), "AUTO 需要至少一个启用的手动节点") {
		t.Fatalf("expected AUTO target guard, got %v", err)
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
	app.Stdin = strings.NewReader("4\n2\n0\n")
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
