package cli

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"minimalist/internal/app"
	"minimalist/internal/config"
	"minimalist/internal/rulesrepo"
	"minimalist/internal/runtime"
	"minimalist/internal/state"
	"minimalist/internal/system"
)

type noopRunner struct{}

func (noopRunner) Run(name string, args ...string) error { return nil }

func (noopRunner) Output(name string, args ...string) (string, string, error) {
	return "", "", nil
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type clearRulesSafeRunner struct{}

func (clearRulesSafeRunner) Run(name string, args ...string) error {
	if name == "iptables" {
		return os.ErrNotExist
	}
	if name == "ip" && len(args) >= 4 && args[0] == "-4" && args[1] == "rule" && args[2] == "del" {
		return os.ErrNotExist
	}
	return nil
}

func (clearRulesSafeRunner) Output(name string, args ...string) (string, string, error) {
	return "", "", nil
}

type recordedCommand struct {
	name string
	args []string
}

type recordingRunner struct {
	calls *[]recordedCommand
}

func (r recordingRunner) Run(name string, args ...string) error {
	*r.calls = append(*r.calls, recordedCommand{name: name, args: append([]string{}, args...)})
	if name == "iptables" {
		return os.ErrNotExist
	}
	if name == "ip" && len(args) >= 4 && args[0] == "-4" && args[1] == "rule" && args[2] == "del" {
		return os.ErrNotExist
	}
	return nil
}

func (r recordingRunner) Output(name string, args ...string) (string, string, error) {
	return "", "", nil
}

type applyRulesRecordingRunner struct {
	calls *[]recordedCommand
}

func (r applyRulesRecordingRunner) Run(name string, args ...string) error {
	*r.calls = append(*r.calls, recordedCommand{name: name, args: append([]string{}, args...)})
	if name == "iptables" {
		if len(args) >= 5 && (args[4] == "-S" || args[4] == "-C") {
			return os.ErrNotExist
		}
		return nil
	}
	if name == "ip" && len(args) >= 4 && args[0] == "-4" && args[1] == "rule" && args[2] == "del" {
		return os.ErrNotExist
	}
	return nil
}

func (r applyRulesRecordingRunner) Output(name string, args ...string) (string, string, error) {
	return "", "", nil
}

func newCLIApp(t *testing.T) (*app.App, *bytes.Buffer) {
	t.Helper()
	root := t.TempDir()
	stdout := &bytes.Buffer{}
	a := &app.App{
		Paths: runtime.Paths{
			ConfigDir:   filepath.Join(root, "etc"),
			DataDir:     filepath.Join(root, "var"),
			RuntimeDir:  filepath.Join(root, "runtime"),
			InstallDir:  filepath.Join(root, "install"),
			BinPath:     filepath.Join(root, "bin", "minimalist"),
			ServiceUnit: filepath.Join(root, "systemd", "minimalist.service"),
			SysctlPath:  filepath.Join(root, "sysctl", "99-minimalist-router.conf"),
		},
		Runner: system.CommandRunner(noopRunner{}),
		Stdout: stdout,
		Stderr: &bytes.Buffer{},
	}
	mustSeedCLIRuntimeAssets(t, a.Paths)
	return a, stdout
}

func mustSeedCLIRuntimeAssets(t *testing.T, paths runtime.Paths) {
	t.Helper()
	if err := os.MkdirAll(paths.UIPath(), 0o755); err != nil {
		t.Fatalf("mkdir runtime ui: %v", err)
	}
	if err := os.WriteFile(paths.CountryMMDBPath(), []byte("mmdb"), 0o640); err != nil {
		t.Fatalf("write runtime mmdb: %v", err)
	}
	if err := os.WriteFile(paths.GeoSitePath(), []byte("geosite"), 0o640); err != nil {
		t.Fatalf("write runtime geosite: %v", err)
	}
}

func hasRecordedRunnerCall(calls []recordedCommand, name string, want ...string) bool {
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

func assertOnlyCutoverPreflightRunnerCalls(t *testing.T, calls []recordedCommand) {
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

func setCLIPathsEnv(t *testing.T) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("MINIMALIST_CONFIG_DIR", filepath.Join(root, "etc"))
	t.Setenv("MINIMALIST_DATA_DIR", filepath.Join(root, "var"))
	t.Setenv("MINIMALIST_RUNTIME_DIR", filepath.Join(root, "runtime"))
	t.Setenv("MINIMALIST_INSTALL_DIR", filepath.Join(root, "install"))
	t.Setenv("MINIMALIST_BIN_PATH", filepath.Join(root, "bin", "minimalist"))
	t.Setenv("MINIMALIST_SERVICE_UNIT", filepath.Join(root, "systemd", "minimalist.service"))
	t.Setenv("MINIMALIST_SYSCTL_PATH", filepath.Join(root, "sysctl", "99-minimalist-router.conf"))
	if err := os.MkdirAll(filepath.Join(root, "runtime", "ui"), 0o755); err != nil {
		t.Fatalf("mkdir runtime ui: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "runtime", "Country.mmdb"), []byte("mmdb"), 0o640); err != nil {
		t.Fatalf("write runtime mmdb: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "runtime", "GeoSite.dat"), []byte("geosite"), 0o640); err != nil {
		t.Fatalf("write runtime geosite: %v", err)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	oldStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stdout: %v", err)
	}
	os.Stdout = writer
	defer func() {
		os.Stdout = oldStdout
	}()
	fn()
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	body, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	return string(body)
}

func withStdinFile(t *testing.T, content string, fn func()) {
	t.Helper()
	file, err := os.CreateTemp(t.TempDir(), "stdin-*")
	if err != nil {
		t.Fatalf("create stdin file: %v", err)
	}
	if _, err := file.WriteString(content); err != nil {
		t.Fatalf("write stdin file: %v", err)
	}
	if _, err := file.Seek(0, 0); err != nil {
		t.Fatalf("seek stdin file: %v", err)
	}
	oldStdin := os.Stdin
	os.Stdin = file
	defer func() {
		os.Stdin = oldStdin
		_ = file.Close()
	}()
	fn()
}

func TestIsTTYReturnsFalseForRegularAndClosedStdin(t *testing.T) {
	oldStdin := os.Stdin
	defer func() { os.Stdin = oldStdin }()

	file, err := os.CreateTemp(t.TempDir(), "stdin-*")
	if err != nil {
		t.Fatalf("create stdin file: %v", err)
	}
	os.Stdin = file
	if isTTY() {
		t.Fatalf("expected regular file stdin to be non-tty")
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close stdin file: %v", err)
	}
	if isTTY() {
		t.Fatalf("expected closed stdin to be non-tty")
	}
}

func mustImportNode(t *testing.T, a *app.App, uri string) {
	t.Helper()
	a.Stdin = strings.NewReader(uri + "\n")
	if err := a.ImportLinks(); err != nil {
		t.Fatalf("import node: %v", err)
	}
}

func expectRootOrSpecificError(t *testing.T, err error, want string) bool {
	t.Helper()
	if os.Geteuid() != 0 {
		if err == nil || !strings.Contains(err.Error(), "请用 root 运行") {
			t.Fatalf("expected root error, got %v", err)
		}
		return false
	}
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("expected %q, got %v", want, err)
	}
	return true
}

func TestRunRulesRepoRequiresSubcommand(t *testing.T) {
	a, _ := newCLIApp(t)
	err := runRulesRepo(a, nil)
	if err == nil || !strings.Contains(err.Error(), "usage: minimalist rules-repo") {
		t.Fatalf("expected usage error, got %v", err)
	}
}

func TestRunNodesUsageErrorThroughRun(t *testing.T) {
	err := Run([]string{"nodes"})
	if err == nil || !strings.Contains(err.Error(), "usage: minimalist nodes list|test|rename|enable|disable|remove ...") {
		t.Fatalf("expected nodes usage error, got %v", err)
	}
}

func TestRunSubscriptionsUsageErrorThroughRun(t *testing.T) {
	err := Run([]string{"subscriptions"})
	if err == nil || !strings.Contains(err.Error(), "usage: minimalist subscriptions list|add|enable|disable|remove|update ...") {
		t.Fatalf("expected subscriptions usage error, got %v", err)
	}
}

func TestRunRulesUsageErrorThroughRun(t *testing.T) {
	err := Run([]string{"rules"})
	if err == nil || !strings.Contains(err.Error(), "usage: minimalist rules list|add|remove ...") {
		t.Fatalf("expected rules usage error, got %v", err)
	}
}

func TestRunACLUsageErrorThroughRun(t *testing.T) {
	err := Run([]string{"acl"})
	if err == nil || !strings.Contains(err.Error(), "usage: minimalist acl list|add|remove ...") {
		t.Fatalf("expected acl usage error, got %v", err)
	}
}

func TestRunRulesRepoUsageErrorThroughRun(t *testing.T) {
	err := Run([]string{"rules-repo"})
	if err == nil || !strings.Contains(err.Error(), "usage: minimalist rules-repo summary|entries|find|add|remove|remove-index ...") {
		t.Fatalf("expected rules-repo usage error, got %v", err)
	}
}

func TestRunNodesIndexErrorThroughRun(t *testing.T) {
	err := Run([]string{"nodes", "rename", "bad", "x"})
	if err == nil || !strings.Contains(err.Error(), "invalid syntax") {
		t.Fatalf("expected nodes index error, got %v", err)
	}
}

func TestRunSubscriptionsIndexErrorThroughRun(t *testing.T) {
	err := Run([]string{"subscriptions", "enable", "bad"})
	if err == nil || !strings.Contains(err.Error(), "invalid syntax") {
		t.Fatalf("expected subscriptions index error, got %v", err)
	}
}

func TestRunRulesIndexErrorThroughRun(t *testing.T) {
	err := Run([]string{"rules", "remove", "bad"})
	if err == nil || !strings.Contains(err.Error(), "invalid syntax") {
		t.Fatalf("expected rules index error, got %v", err)
	}
}

func TestRunACLIndexErrorThroughRun(t *testing.T) {
	err := Run([]string{"acl", "remove", "bad"})
	if err == nil || !strings.Contains(err.Error(), "invalid syntax") {
		t.Fatalf("expected acl index error, got %v", err)
	}
}

func TestRunRulesRepoRemoveIndexErrorThroughRun(t *testing.T) {
	err := Run([]string{"rules-repo", "remove-index", "defaults", "bad"})
	if err == nil || !strings.Contains(err.Error(), "invalid syntax") {
		t.Fatalf("expected rules-repo index error, got %v", err)
	}
}

func TestRunRulesRepoUsageAndIndexErrors(t *testing.T) {
	a, _ := newCLIApp(t)
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"entries"}, "usage: minimalist rules-repo entries <ruleset> [keyword]"},
		{[]string{"find"}, "usage: minimalist rules-repo find <keyword>"},
		{[]string{"add", "pt"}, "usage: minimalist rules-repo add <ruleset> <value>"},
		{[]string{"remove", "pt"}, "usage: minimalist rules-repo remove <ruleset> <value>"},
		{[]string{"remove-index", "pt"}, "usage: minimalist rules-repo remove-index <ruleset> <index>"},
		{[]string{"remove-index", "pt", "bad"}, `strconv.Atoi: parsing "bad"`},
		{[]string{"remove-index", "pt", "9999"}, "entry index out of range"},
	} {
		err := runRulesRepo(a, tc.args)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("args=%v want %q got %v", tc.args, tc.want, err)
		}
	}
}

func TestRunRulesRepoUnknownSubcommand(t *testing.T) {
	a, _ := newCLIApp(t)
	err := runRulesRepo(a, []string{"unknown"})
	if err == nil || !strings.Contains(err.Error(), "unknown rules-repo command: unknown") {
		t.Fatalf("expected unknown rules-repo command error, got %v", err)
	}
}

func TestRunRulesRepoSummaryAndEntries(t *testing.T) {
	a, stdout := newCLIApp(t)
	if err := runRulesRepo(a, []string{"summary"}); err != nil {
		t.Fatalf("summary: %v", err)
	}
	if err := runRulesRepo(a, []string{"entries", "pt", "smzdm"}); err != nil {
		t.Fatalf("entries: %v", err)
	}
	output := stdout.String()
	for _, needle := range []string{"规则仓库:", "- pt:", "总规则数:", "1\tsmzdm.com"} {
		if !strings.Contains(output, needle) {
			t.Fatalf("missing %q in output:\n%s", needle, output)
		}
	}
}

func TestRunRulesRepoAddAndRemoveIndex(t *testing.T) {
	a, stdout := newCLIApp(t)
	if err := runRulesRepo(a, []string{"add", "pt", "example-pt.test"}); err != nil {
		t.Fatalf("add: %v", err)
	}
	stdout.Reset()
	if err := runRulesRepo(a, []string{"entries", "pt"}); err != nil {
		t.Fatalf("entries after add: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	last := strings.SplitN(lines[len(lines)-1], "\t", 2)
	if len(last) != 2 || last[1] != "example-pt.test" {
		t.Fatalf("expected added entry at tail, got:\n%s", stdout.String())
	}
	index, err := strconv.Atoi(last[0])
	if err != nil {
		t.Fatalf("parse remove index: %v", err)
	}
	if err := runRulesRepo(a, []string{"remove-index", "pt", strconv.Itoa(index)}); err != nil {
		t.Fatalf("remove-index: %v", err)
	}
	stdout.Reset()
	if err := runRulesRepo(a, []string{"entries", "pt", "example-pt"}); err != nil {
		t.Fatalf("entries after remove: %v", err)
	}
	if strings.Contains(stdout.String(), "example-pt.test") {
		t.Fatalf("expected entry removal, got:\n%s", stdout.String())
	}
}

func TestRunNodesRenameToggleAndRemove(t *testing.T) {
	a, stdout := newCLIApp(t)
	mustImportNode(t, a, "trojan://password@example.org:443?security=tls#cli-node")
	if err := runNodes(a, []string{"list"}); err != nil {
		t.Fatalf("list nodes: %v", err)
	}
	if !strings.Contains(stdout.String(), "1\tcli-node\t0\tmanual") {
		t.Fatalf("unexpected node list:\n%s", stdout.String())
	}
	if err := runNodes(a, []string{"rename", "1", "cli-node-renamed"}); err != nil {
		t.Fatalf("rename node: %v", err)
	}
	if err := runNodes(a, []string{"enable", "1"}); err != nil {
		t.Fatalf("enable node: %v", err)
	}
	stdout.Reset()
	if err := runNodes(a, []string{"list"}); err != nil {
		t.Fatalf("list nodes after enable: %v", err)
	}
	if !strings.Contains(stdout.String(), "1\tcli-node-renamed\t1\tmanual") {
		t.Fatalf("unexpected node list after enable:\n%s", stdout.String())
	}
	if err := runNodes(a, []string{"disable", "1"}); err != nil {
		t.Fatalf("disable node: %v", err)
	}
	if err := runNodes(a, []string{"remove", "1"}); err != nil {
		t.Fatalf("remove node: %v", err)
	}
	stdout.Reset()
	if err := runNodes(a, []string{"list"}); err != nil {
		t.Fatalf("list nodes after remove: %v", err)
	}
	if strings.TrimSpace(stdout.String()) != "" {
		t.Fatalf("expected empty node list, got:\n%s", stdout.String())
	}
}

func TestRunNodesUsageAndIndexErrors(t *testing.T) {
	a, _ := newCLIApp(t)
	for _, tc := range []struct {
		args []string
		want string
	}{
		{nil, "usage: minimalist nodes list|test|rename|enable|disable|remove ..."},
		{[]string{"rename", "1"}, "usage: minimalist nodes rename <index> <new-name>"},
		{[]string{"enable"}, "usage: minimalist nodes enable|disable <index>"},
		{[]string{"disable", "bad"}, `strconv.Atoi: parsing "bad"`},
		{[]string{"remove"}, "usage: minimalist nodes remove <index>"},
		{[]string{"remove", "1"}, "node index out of range"},
	} {
		err := runNodes(a, tc.args)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("args=%v want %q got %v", tc.args, tc.want, err)
		}
	}
}

func TestRunNodesUnknownSubcommand(t *testing.T) {
	a, _ := newCLIApp(t)
	err := runNodes(a, []string{"unknown"})
	if err == nil || !strings.Contains(err.Error(), "unknown nodes command: unknown") {
		t.Fatalf("expected unknown nodes command error, got %v", err)
	}
}

func TestRunSubscriptionsAddDisableAndRemove(t *testing.T) {
	a, stdout := newCLIApp(t)
	if err := runSubscriptions(a, []string{"add", "cli-sub", "https://subscription.example.com/cli.txt"}); err != nil {
		t.Fatalf("add subscription: %v", err)
	}
	if err := runSubscriptions(a, []string{"list"}); err != nil {
		t.Fatalf("list subscriptions: %v", err)
	}
	if !strings.Contains(stdout.String(), "1\tcli-sub\thttps://subscription.example.com/cli.txt\t1\t\t0\t") {
		t.Fatalf("unexpected subscription list:\n%s", stdout.String())
	}
	if err := runSubscriptions(a, []string{"disable", "1"}); err != nil {
		t.Fatalf("disable subscription: %v", err)
	}
	stdout.Reset()
	if err := runSubscriptions(a, []string{"list"}); err != nil {
		t.Fatalf("list subscriptions after disable: %v", err)
	}
	if !strings.Contains(stdout.String(), "1\tcli-sub\thttps://subscription.example.com/cli.txt\t0\t\t0\t") {
		t.Fatalf("unexpected disabled subscription list:\n%s", stdout.String())
	}
	if err := runSubscriptions(a, []string{"remove", "1"}); err != nil {
		t.Fatalf("remove subscription: %v", err)
	}
	stdout.Reset()
	if err := runSubscriptions(a, []string{"list"}); err != nil {
		t.Fatalf("list subscriptions after remove: %v", err)
	}
	if strings.TrimSpace(stdout.String()) != "" {
		t.Fatalf("expected empty subscription list, got:\n%s", stdout.String())
	}
}

func TestRunSubscriptionsUpdateRefreshesEnabledEntries(t *testing.T) {
	a, _ := newCLIApp(t)
	if err := runSubscriptions(a, []string{"add", "cli-update", "https://subscription.example.com/cli-update.txt"}); err != nil {
		t.Fatalf("add subscription: %v", err)
	}
	a.Client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.String() != "https://subscription.example.com/cli-update.txt" {
				t.Fatalf("unexpected subscription fetch: %s", req.URL.String())
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("trojan://password@example.org:443?security=tls#cli-sub-node\n")),
				Header:     make(http.Header),
			}, nil
		}),
	}
	if err := runSubscriptions(a, []string{"update"}); err != nil {
		t.Fatalf("update subscriptions: %v", err)
	}
	st, err := state.Load(a.Paths.StatePath())
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if st.Subscriptions[0].Enumeration.LastCount != 1 || len(st.Nodes) != 1 || st.Nodes[0].Name != "cli-sub-node" {
		t.Fatalf("unexpected subscription update state: %+v", st)
	}
}

func TestRunSubscriptionsUsageAndIndexErrors(t *testing.T) {
	a, _ := newCLIApp(t)
	for _, tc := range []struct {
		args []string
		want string
	}{
		{nil, "usage: minimalist subscriptions list|add|enable|disable|remove|update ..."},
		{[]string{"add", "demo"}, "usage: minimalist subscriptions add <name> <url>"},
		{[]string{"enable"}, "usage: minimalist subscriptions enable|disable <index>"},
		{[]string{"disable", "bad"}, `strconv.Atoi: parsing "bad"`},
		{[]string{"remove"}, "usage: minimalist subscriptions remove <index>"},
		{[]string{"remove", "1"}, "subscription index out of range"},
	} {
		err := runSubscriptions(a, tc.args)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("args=%v want %q got %v", tc.args, tc.want, err)
		}
	}
}

func TestRunSubscriptionsUnknownSubcommand(t *testing.T) {
	a, _ := newCLIApp(t)
	err := runSubscriptions(a, []string{"unknown"})
	if err == nil || !strings.Contains(err.Error(), "unknown subscriptions command: unknown") {
		t.Fatalf("expected unknown subscriptions command error, got %v", err)
	}
}

func TestRunRulesAndACLAddListRemove(t *testing.T) {
	a, stdout := newCLIApp(t)
	if err := runRules(a, false, []string{"add", "domain", "example.com", "DIRECT"}); err != nil {
		t.Fatalf("add rule: %v", err)
	}
	if err := runRules(a, false, []string{"list"}); err != nil {
		t.Fatalf("list rules: %v", err)
	}
	if !strings.Contains(stdout.String(), "1\tDOMAIN,example.com,DIRECT") {
		t.Fatalf("unexpected rule list:\n%s", stdout.String())
	}
	stdout.Reset()
	if err := runRules(a, true, []string{"add", "src-cidr", "192.168.2.10/32", "DIRECT"}); err != nil {
		t.Fatalf("add acl: %v", err)
	}
	if err := runRules(a, true, []string{"list"}); err != nil {
		t.Fatalf("list acl: %v", err)
	}
	if !strings.Contains(stdout.String(), "1\tSRC-IP-CIDR,192.168.2.10/32,DIRECT") {
		t.Fatalf("unexpected acl list:\n%s", stdout.String())
	}
	if err := runRules(a, false, []string{"remove", "1"}); err != nil {
		t.Fatalf("remove rule: %v", err)
	}
	if err := runRules(a, true, []string{"remove", "1"}); err != nil {
		t.Fatalf("remove acl: %v", err)
	}
	stdout.Reset()
	if err := runRules(a, false, []string{"list"}); err != nil {
		t.Fatalf("list rules after remove: %v", err)
	}
	if err := runRules(a, true, []string{"list"}); err != nil {
		t.Fatalf("list acl after remove: %v", err)
	}
	if strings.TrimSpace(stdout.String()) != "" {
		t.Fatalf("expected empty rule/acl output, got:\n%s", stdout.String())
	}
}

func TestRunRulesAndACLUsageAndIndexErrors(t *testing.T) {
	a, _ := newCLIApp(t)
	for _, tc := range []struct {
		acl  bool
		args []string
		want string
	}{
		{false, nil, "usage: minimalist rules list|add|remove ..."},
		{false, []string{"add", "domain", "example.com"}, "usage: minimalist rules add <kind> <pattern> <target>"},
		{false, []string{"remove"}, "usage: minimalist rules remove <index>"},
		{false, []string{"remove", "1"}, "rule index out of range"},
		{true, nil, "usage: minimalist acl list|add|remove ..."},
		{true, []string{"add", "src-cidr", "192.168.1.1/32"}, "usage: minimalist acl add <kind> <pattern> <target>"},
		{true, []string{"remove"}, "usage: minimalist acl remove <index>"},
		{true, []string{"remove", "1"}, "rule index out of range"},
	} {
		err := runRules(a, tc.acl, tc.args)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("acl=%t args=%v want %q got %v", tc.acl, tc.args, tc.want, err)
		}
	}
}

func TestRunRulesAndACLUnknownSubcommand(t *testing.T) {
	a, _ := newCLIApp(t)
	for _, tc := range []struct {
		acl  bool
		args []string
		want string
	}{
		{false, []string{"unknown"}, "unknown rules command: unknown"},
		{true, []string{"unknown"}, "unknown acl command: unknown"},
	} {
		err := runRules(a, tc.acl, tc.args)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("acl=%t args=%v want %q got %v", tc.acl, tc.args, tc.want, err)
		}
	}
}

func TestRunWithoutArgsOnNonTTYPrintsUsage(t *testing.T) {
	setCLIPathsEnv(t)
	withStdinFile(t, "", func() {
		output := captureStdout(t, func() {
			if err := Run(nil); err != nil {
				t.Fatalf("run without args: %v", err)
			}
		})
		if !strings.Contains(output, "minimalist commands:") {
			t.Fatalf("expected usage output, got:\n%s", output)
		}
	})
}

func TestRunHelpPrintsUsage(t *testing.T) {
	setCLIPathsEnv(t)
	output := captureStdout(t, func() {
		if err := Run([]string{"--help"}); err != nil {
			t.Fatalf("run help: %v", err)
		}
	})
	for _, needle := range []string{
		"minimalist commands:",
		"minimalist core-upgrade-alpha",
		"  minimalist verify-runtime-assets\n",
		"  minimalist nodes list|test|rename|enable|disable|remove\n",
		"minimalist rules-repo summary|entries|find|add|remove|remove-index",
	} {
		if !strings.Contains(output, needle) {
			t.Fatalf("missing %q in help output:\n%s", needle, output)
		}
	}
}

func TestRunShortHelpPrintsUsage(t *testing.T) {
	setCLIPathsEnv(t)
	output := captureStdout(t, func() {
		if err := Run([]string{"-h"}); err != nil {
			t.Fatalf("run short help: %v", err)
		}
	})
	if !strings.Contains(output, "minimalist commands:") {
		t.Fatalf("expected short help output, got:\n%s", output)
	}
}

func TestRunHelpAliasPrintsUsage(t *testing.T) {
	setCLIPathsEnv(t)
	output := captureStdout(t, func() {
		if err := Run([]string{"help"}); err != nil {
			t.Fatalf("run help alias: %v", err)
		}
	})
	if !strings.Contains(output, "minimalist commands:") {
		t.Fatalf("expected help alias output, got:\n%s", output)
	}
}

func TestRunUnknownCommandReturnsError(t *testing.T) {
	setCLIPathsEnv(t)
	err := Run([]string{"unknown-subcommand"})
	if err == nil || !strings.Contains(err.Error(), "unknown command: unknown-subcommand") {
		t.Fatalf("expected unknown command error, got %v", err)
	}
}

func TestRunDispatchesRulesRepoSummary(t *testing.T) {
	setCLIPathsEnv(t)
	output := captureStdout(t, func() {
		if err := Run([]string{"rules-repo", "summary"}); err != nil {
			t.Fatalf("run rules-repo summary: %v", err)
		}
	})
	for _, needle := range []string{"规则仓库:", "- pt:", "总规则数:"} {
		if !strings.Contains(output, needle) {
			t.Fatalf("missing %q in summary output:\n%s", needle, output)
		}
	}
}

func TestRunDispatchesStatusThroughRun(t *testing.T) {
	setCLIPathsEnv(t)
	output := captureStdout(t, func() {
		if err := Run([]string{"status"}); err != nil {
			t.Fatalf("run status: %v", err)
		}
	})
	for _, needle := range []string{"项目: minimalist", "服务状态:"} {
		if !strings.Contains(output, needle) {
			t.Fatalf("missing %q in status output:\n%s", needle, output)
		}
	}
}

func TestRunDispatchesRenderConfigThroughRun(t *testing.T) {
	setCLIPathsEnv(t)
	a := app.New()
	mustImportNode(t, a, "trojan://password@example.org:443?security=tls#run-render")
	if err := a.SetNodeEnabled(1, true); err != nil {
		t.Fatalf("enable node: %v", err)
	}
	output := captureStdout(t, func() {
		if err := Run([]string{"render-config"}); err != nil {
			t.Fatalf("run render-config: %v", err)
		}
	})
	if _, err := os.Stat(a.Paths.RuntimeConfig()); err != nil {
		t.Fatalf("expected runtime config: %v", err)
	}
	if !strings.Contains(output, "已生成 "+a.Paths.RuntimeConfig()) {
		t.Fatalf("unexpected render-config output:\n%s", output)
	}
}

func TestRunDispatchesHealthcheckThroughRun(t *testing.T) {
	setCLIPathsEnv(t)
	output := captureStdout(t, func() {
		if err := Run([]string{"healthcheck"}); err != nil {
			t.Fatalf("run healthcheck: %v", err)
		}
	})
	for _, needle := range []string{"mixed-port=7890", "controller-port=19090"} {
		if !strings.Contains(output, needle) {
			t.Fatalf("missing %q in healthcheck output:\n%s", needle, output)
		}
	}
}

func TestRunDispatchesVerifyRuntimeAssetsThroughRun(t *testing.T) {
	setCLIPathsEnv(t)
	if err := Run([]string{"verify-runtime-assets"}); err != nil {
		t.Fatalf("run verify-runtime-assets: %v", err)
	}
}

func TestRunDispatchesRuntimeAuditThroughRun(t *testing.T) {
	setCLIPathsEnv(t)
	output := captureStdout(t, func() {
		if err := Run([]string{"runtime-audit"}); err != nil {
			t.Fatalf("run runtime-audit: %v", err)
		}
	})
	for _, needle := range []string{"alerts-24h:", "alerts-recent:", "fatal-gaps=", "enabled-manual-nodes="} {
		if !strings.Contains(output, needle) {
			t.Fatalf("missing %q in runtime audit output:\n%s", needle, output)
		}
	}
}

func TestRunDispatchesCutoverPreflightThroughRun(t *testing.T) {
	setCLIPathsEnv(t)
	output := captureStdout(t, func() {
		if err := Run([]string{"cutover-preflight"}); err != nil {
			t.Fatalf("run cutover-preflight: %v", err)
		}
	})
	for _, needle := range []string{"cutover-preflight:", "cutover-ready="} {
		if !strings.Contains(output, needle) {
			t.Fatalf("missing %q in cutover preflight output:\n%s", needle, output)
		}
	}
}

func TestRunDispatchesCutoverPlanThroughRun(t *testing.T) {
	setCLIPathsEnv(t)
	output := captureStdout(t, func() {
		if err := Run([]string{"cutover-plan"}); err != nil {
			t.Fatalf("run cutover-plan: %v", err)
		}
	})
	for _, needle := range []string{"cutover-plan:", "next-action:", "rollback:"} {
		if !strings.Contains(output, needle) {
			t.Fatalf("missing %q in cutover plan output:\n%s", needle, output)
		}
	}
}

func TestRunDispatchesImportLinksThroughRun(t *testing.T) {
	setCLIPathsEnv(t)
	withStdinFile(t, "trojan://password@example.org:443?security=tls#run-import\n", func() {
		output := captureStdout(t, func() {
			if err := Run([]string{"import-links"}); err != nil {
				t.Fatalf("run import-links: %v", err)
			}
		})
		if !strings.Contains(output, "已处理 1 条节点") {
			t.Fatalf("unexpected import-links output:\n%s", output)
		}
	})
}

func TestRunDispatchesNodesList(t *testing.T) {
	setCLIPathsEnv(t)
	a := app.New()
	mustImportNode(t, a, "trojan://password@example.org:443?security=tls#run-node")
	output := captureStdout(t, func() {
		if err := Run([]string{"nodes", "list"}); err != nil {
			t.Fatalf("run nodes list: %v", err)
		}
	})
	if !strings.Contains(output, "1\trun-node\t0\tmanual") {
		t.Fatalf("unexpected nodes list output:\n%s", output)
	}
}

func TestRunDispatchesNodesTest(t *testing.T) {
	setCLIPathsEnv(t)
	a := app.New()
	mustImportNode(t, a, "trojan://password@example.org:443?security=tls#run-node-test")
	if err := a.SetNodeEnabled(1, true); err != nil {
		t.Fatalf("enable node: %v", err)
	}
	oldTransport := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/proxies/run-node-test/delay" {
			t.Fatalf("unexpected controller path: %s", req.URL.Path)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"delay":12}`)),
			Header:     make(http.Header),
		}, nil
	})
	t.Cleanup(func() { http.DefaultTransport = oldTransport })
	output := captureStdout(t, func() {
		if err := Run([]string{"nodes", "test"}); err != nil {
			t.Fatalf("run nodes test: %v", err)
		}
	})
	if !strings.Contains(output, "run-node-test\t12ms") {
		t.Fatalf("unexpected nodes test output:\n%s", output)
	}
}

func TestRunWithAppDispatchesNodeManagementSubcommands(t *testing.T) {
	a, stdout := newCLIApp(t)
	mustImportNode(t, a, "trojan://password@example.org:443?security=tls#managed-node")
	if err := runWithApp([]string{"nodes", "rename", "1", "managed-renamed"}, a, false); err != nil {
		t.Fatalf("rename node through dispatcher: %v", err)
	}
	if err := runWithApp([]string{"nodes", "enable", "1"}, a, false); err != nil {
		t.Fatalf("enable node through dispatcher: %v", err)
	}
	stdout.Reset()
	if err := runWithApp([]string{"nodes", "list"}, a, false); err != nil {
		t.Fatalf("list node through dispatcher: %v", err)
	}
	if !strings.Contains(stdout.String(), "1\tmanaged-renamed\t1\tmanual") {
		t.Fatalf("unexpected managed node after enable:\n%s", stdout.String())
	}
	if err := runWithApp([]string{"nodes", "disable", "1"}, a, false); err != nil {
		t.Fatalf("disable node through dispatcher: %v", err)
	}
	if err := runWithApp([]string{"nodes", "remove", "1"}, a, false); err != nil {
		t.Fatalf("remove node through dispatcher: %v", err)
	}
	stdout.Reset()
	if err := runWithApp([]string{"nodes", "list"}, a, false); err != nil {
		t.Fatalf("list nodes after remove through dispatcher: %v", err)
	}
	if strings.TrimSpace(stdout.String()) != "" {
		t.Fatalf("expected empty node list after remove, got:\n%s", stdout.String())
	}
}

func TestRunDispatchesRulesUnknownSubcommand(t *testing.T) {
	setCLIPathsEnv(t)
	err := Run([]string{"rules", "unknown"})
	if err == nil || !strings.Contains(err.Error(), "unknown rules command: unknown") {
		t.Fatalf("expected rules unknown subcommand error, got %v", err)
	}
}

func TestRunDispatchesACLUnknownSubcommand(t *testing.T) {
	setCLIPathsEnv(t)
	err := Run([]string{"acl", "unknown"})
	if err == nil || !strings.Contains(err.Error(), "unknown acl command: unknown") {
		t.Fatalf("expected acl unknown subcommand error, got %v", err)
	}
}

func TestRunDispatchesNodesUnknownSubcommand(t *testing.T) {
	setCLIPathsEnv(t)
	err := Run([]string{"nodes", "unknown"})
	if err == nil || !strings.Contains(err.Error(), "unknown nodes command: unknown") {
		t.Fatalf("expected nodes unknown subcommand error, got %v", err)
	}
}

func TestRunDispatchesSubscriptionsUnknownSubcommand(t *testing.T) {
	setCLIPathsEnv(t)
	err := Run([]string{"subscriptions", "unknown"})
	if err == nil || !strings.Contains(err.Error(), "unknown subscriptions command: unknown") {
		t.Fatalf("expected subscriptions unknown subcommand error, got %v", err)
	}
}

func TestRunDispatchesRulesRepoUnknownSubcommand(t *testing.T) {
	setCLIPathsEnv(t)
	err := Run([]string{"rules-repo", "unknown"})
	if err == nil || !strings.Contains(err.Error(), "unknown rules-repo command: unknown") {
		t.Fatalf("expected rules-repo unknown subcommand error, got %v", err)
	}
}

func TestRunDispatchesSubscriptionsList(t *testing.T) {
	setCLIPathsEnv(t)
	a := app.New()
	if err := a.AddSubscription("run-sub", "https://subscription.example.com/run.txt", true); err != nil {
		t.Fatalf("add subscription: %v", err)
	}
	output := captureStdout(t, func() {
		if err := Run([]string{"subscriptions", "list"}); err != nil {
			t.Fatalf("run subscriptions list: %v", err)
		}
	})
	if !strings.Contains(output, "1\trun-sub\thttps://subscription.example.com/run.txt\t1\t\t0\t") {
		t.Fatalf("unexpected subscriptions list output:\n%s", output)
	}
}

func TestRunDispatchesRulesRepoEntries(t *testing.T) {
	setCLIPathsEnv(t)
	output := captureStdout(t, func() {
		if err := Run([]string{"rules-repo", "entries", "pt", "smzdm"}); err != nil {
			t.Fatalf("run rules-repo entries: %v", err)
		}
	})
	if !strings.Contains(output, "1\tsmzdm.com") {
		t.Fatalf("unexpected rules-repo entries output:\n%s", output)
	}
}

func TestRunDispatchesRulesRepoFindThroughRun(t *testing.T) {
	setCLIPathsEnv(t)
	output := captureStdout(t, func() {
		if err := Run([]string{"rules-repo", "find", "google"}); err != nil {
			t.Fatalf("run rules-repo find: %v", err)
		}
	})
	if !strings.Contains(output, "keyword=google") || !strings.Contains(output, "matched=") {
		t.Fatalf("unexpected rules-repo find output:\n%s", output)
	}
}

func TestRunDispatchesRulesRepoAddRemoveAndRemoveIndex(t *testing.T) {
	setCLIPathsEnv(t)
	a := app.New()

	if err := Run([]string{"rules-repo", "add", "pt", "cli-run.example.com"}); err != nil {
		t.Fatalf("run rules-repo add: %v", err)
	}
	lines, err := rulesrepo.ListEntries(a.Paths.RulesRepoPath(), "pt", "cli-run")
	if err != nil {
		t.Fatalf("list entries after add: %v", err)
	}
	if len(lines) == 0 || !strings.Contains(lines[0], "cli-run.example.com") {
		t.Fatalf("expected added entry, got %#v", lines)
	}

	if err := Run([]string{"rules-repo", "remove", "pt", "cli-run.example.com"}); err != nil {
		t.Fatalf("run rules-repo remove: %v", err)
	}
	lines, err = rulesrepo.ListEntries(a.Paths.RulesRepoPath(), "pt", "cli-run")
	if err != nil {
		t.Fatalf("list entries after remove: %v", err)
	}
	if len(lines) != 0 {
		t.Fatalf("expected entry removed, got %#v", lines)
	}

	if err := Run([]string{"rules-repo", "add", "pt", "cli-run-index.example.com"}); err != nil {
		t.Fatalf("run rules-repo add for remove-index: %v", err)
	}
	lines, err = rulesrepo.ListEntries(a.Paths.RulesRepoPath(), "pt", "cli-run-index")
	if err != nil {
		t.Fatalf("list entries after re-add: %v", err)
	}
	if len(lines) == 0 {
		t.Fatalf("expected indexed entry after re-add")
	}
	indexText, _, ok := strings.Cut(lines[0], "\t")
	if !ok {
		t.Fatalf("unexpected indexed entry: %q", lines[0])
	}
	index, err := strconv.Atoi(indexText)
	if err != nil {
		t.Fatalf("parse remove-index: %v", err)
	}
	if err := Run([]string{"rules-repo", "remove-index", "pt", strconv.Itoa(index)}); err != nil {
		t.Fatalf("run rules-repo remove-index: %v", err)
	}
	lines, err = rulesrepo.ListEntries(a.Paths.RulesRepoPath(), "pt", "cli-run-index")
	if err != nil {
		t.Fatalf("list entries after remove-index: %v", err)
	}
	if len(lines) != 0 {
		t.Fatalf("expected entry removed by index, got %#v", lines)
	}
}

func TestRunDispatchesRulesList(t *testing.T) {
	setCLIPathsEnv(t)
	a := app.New()
	if err := a.AddRule(false, "domain", "example.com", "DIRECT"); err != nil {
		t.Fatalf("add rule: %v", err)
	}
	output := captureStdout(t, func() {
		if err := Run([]string{"rules", "list"}); err != nil {
			t.Fatalf("run rules list: %v", err)
		}
	})
	if !strings.Contains(output, "1\tDOMAIN,example.com,DIRECT") {
		t.Fatalf("unexpected rules list output:\n%s", output)
	}
}

func TestRunDispatchesACLList(t *testing.T) {
	setCLIPathsEnv(t)
	a := app.New()
	if err := a.AddRule(true, "src-cidr", "192.168.2.10/32", "DIRECT"); err != nil {
		t.Fatalf("add acl: %v", err)
	}
	output := captureStdout(t, func() {
		if err := Run([]string{"acl", "list"}); err != nil {
			t.Fatalf("run acl list: %v", err)
		}
	})
	if !strings.Contains(output, "1\tSRC-IP-CIDR,192.168.2.10/32,DIRECT") {
		t.Fatalf("unexpected acl list output:\n%s", output)
	}
}

func TestRunDispatchesSubscriptionsUpdate(t *testing.T) {
	setCLIPathsEnv(t)
	a := app.New()
	a.Client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader("trojan://password@example.org:443?security=tls#run-sub\n")),
				Header:     make(http.Header),
			}, nil
		}),
	}
	if err := a.AddSubscription("run-sub", "https://subscription.example.com/run.txt", true); err != nil {
		t.Fatalf("add subscription: %v", err)
	}
	output := captureStdout(t, func() {
		if err := runWithApp([]string{"subscriptions", "update"}, a, false); err != nil {
			t.Fatalf("run subscriptions update: %v", err)
		}
	})
	if strings.Contains(output, "error") {
		t.Fatalf("unexpected subscriptions update output:\n%s", output)
	}
}

func TestRunDispatchesMenuThroughRun(t *testing.T) {
	setCLIPathsEnv(t)
	output := captureStdout(t, func() {
		oldStdin := os.Stdin
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatalf("pipe stdin: %v", err)
		}
		_, _ = w.WriteString("0\n")
		_ = w.Close()
		os.Stdin = r
		defer func() { os.Stdin = oldStdin }()
		if err := Run([]string{"menu"}); err != nil {
			t.Fatalf("run menu: %v", err)
		}
	})
	if !strings.Contains(output, "1) 状态") || !strings.Contains(output, "0) 退出") {
		t.Fatalf("unexpected menu output:\n%s", output)
	}
}

func TestRunDispatchesSetupThroughRun(t *testing.T) {
	setCLIPathsEnv(t)
	a, _ := newCLIApp(t)
	a.Runner = system.CommandRunner(noopRunner{})
	err := runWithApp([]string{"setup"}, a, false)
	if err != nil {
		t.Fatalf("run setup: %v", err)
	}
}

func TestRunDispatchesClearRulesThroughRun(t *testing.T) {
	a, _ := newCLIApp(t)
	a.Runner = system.CommandRunner(clearRulesSafeRunner{})
	err := runWithApp([]string{"clear-rules"}, a, false)
	if os.Geteuid() != 0 {
		if err == nil || !strings.Contains(err.Error(), "请用 root 运行") {
			t.Fatalf("expected root error, got %v", err)
		}
		return
	}
	if err != nil {
		t.Fatalf("run clear-rules: %v", err)
	}
}

func TestRunDispatchesShowSecret(t *testing.T) {
	setCLIPathsEnv(t)
	cfg, err := config.Ensure(runtime.DefaultPaths().ConfigPath())
	if err != nil {
		t.Fatalf("ensure config: %v", err)
	}
	cfg.Controller.Secret = "run-secret"
	if err := config.Save(runtime.DefaultPaths().ConfigPath(), cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	output := captureStdout(t, func() {
		if err := Run([]string{"show-secret"}); err != nil {
			t.Fatalf("run show-secret: %v", err)
		}
	})
	if !strings.Contains(output, "run-secret") {
		t.Fatalf("unexpected show-secret output:\n%s", output)
	}
}

func TestRunWithAppOnTTYWithoutArgsEntersMenu(t *testing.T) {
	a, stdout := newCLIApp(t)
	a.Stdin = strings.NewReader("0\n")
	if err := runWithApp(nil, a, true); err != nil {
		t.Fatalf("run with tty menu: %v", err)
	}
	output := stdout.String()
	for _, needle := range []string{"1) 状态", "0) 退出"} {
		if !strings.Contains(output, needle) {
			t.Fatalf("missing %q in menu output:\n%s", needle, output)
		}
	}
}

func TestRunWithAppDispatchesCoreUpgradeAlpha(t *testing.T) {
	a, _ := newCLIApp(t)
	a.Client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return nil, os.ErrNotExist
		}),
	}
	err := runWithApp([]string{"core-upgrade-alpha"}, a, false)
	if os.Geteuid() != 0 {
		if err == nil || !strings.Contains(err.Error(), "请用 root 运行") {
			t.Fatalf("expected root error, got %v", err)
		}
		return
	}
	if err == nil || !strings.Contains(err.Error(), os.ErrNotExist.Error()) {
		t.Fatalf("expected injected client error, got %v", err)
	}
}

func TestRunWithAppDispatchesSetup(t *testing.T) {
	a, stdout := newCLIApp(t)
	err := runWithApp([]string{"setup"}, a, false)
	if os.Geteuid() != 0 {
		if err == nil || !strings.Contains(err.Error(), "请用 root 运行") {
			t.Fatalf("expected root error, got %v", err)
		}
		return
	}
	if err != nil {
		t.Fatalf("run setup: %v", err)
	}
	for _, path := range []string{a.Paths.ServiceUnit, a.Paths.SysctlPath} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected generated file %s: %v", path, err)
		}
	}
	if !strings.Contains(stdout.String(), "部署完成") {
		t.Fatalf("unexpected setup output:\n%s", stdout.String())
	}
}

func TestRunWithAppDispatchesSetupPropagatesManifestError(t *testing.T) {
	a, _ := newCLIApp(t)
	var calls []recordedCommand
	a.Runner = system.CommandRunner(recordingRunner{calls: &calls})
	if err := os.MkdirAll(filepath.Dir(a.Paths.RulesRepoPath()), 0o755); err != nil {
		t.Fatalf("mkdir rules repo dir: %v", err)
	}
	if err := os.WriteFile(a.Paths.RulesRepoPath(), []byte("rulesets: [\n"), 0o640); err != nil {
		t.Fatalf("write invalid manifest: %v", err)
	}
	err := runWithApp([]string{"setup"}, a, false)
	if !expectRootOrSpecificError(t, err, "parse manifest") {
		return
	}
	assertOnlyCutoverPreflightRunnerCalls(t, calls)
}

func TestRunWithAppDispatchesSetupPropagatesManualProviderFailure(t *testing.T) {
	a, _ := newCLIApp(t)
	var calls []recordedCommand
	a.Runner = system.CommandRunner(recordingRunner{calls: &calls})
	if err := os.MkdirAll(a.Paths.ManualProvider(), 0o755); err != nil {
		t.Fatalf("mkdir blocking manual provider path: %v", err)
	}
	err := runWithApp([]string{"setup"}, a, false)
	if !expectRootOrSpecificError(t, err, "is a directory") {
		return
	}
	assertOnlyCutoverPreflightRunnerCalls(t, calls)
}

func TestRunWithAppDispatchesSetupPropagatesRuntimeConfigFailure(t *testing.T) {
	a, _ := newCLIApp(t)
	var calls []recordedCommand
	a.Runner = system.CommandRunner(recordingRunner{calls: &calls})
	if err := os.MkdirAll(a.Paths.RuntimeConfig(), 0o755); err != nil {
		t.Fatalf("mkdir blocking runtime config path: %v", err)
	}
	err := runWithApp([]string{"setup"}, a, false)
	if !expectRootOrSpecificError(t, err, "is a directory") {
		return
	}
	assertOnlyCutoverPreflightRunnerCalls(t, calls)
}

func TestRunWithAppDispatchesRenderConfig(t *testing.T) {
	a, stdout := newCLIApp(t)
	mustImportNode(t, a, "trojan://password@example.org:443?security=tls#render-dispatch")
	if err := a.SetNodeEnabled(1, true); err != nil {
		t.Fatalf("enable node: %v", err)
	}
	if err := runWithApp([]string{"render-config"}, a, false); err != nil {
		t.Fatalf("run render-config: %v", err)
	}
	if _, err := os.Stat(a.Paths.RuntimeConfig()); err != nil {
		t.Fatalf("expected runtime config: %v", err)
	}
	if !strings.Contains(stdout.String(), "已生成 "+a.Paths.RuntimeConfig()) {
		t.Fatalf("unexpected render-config output:\n%s", stdout.String())
	}
}

func TestRunWithAppDispatchesRenderConfigPropagatesRuntimeConfigFailure(t *testing.T) {
	a, _ := newCLIApp(t)
	if err := os.MkdirAll(a.Paths.RuntimeConfig(), 0o755); err != nil {
		t.Fatalf("mkdir blocking runtime config path: %v", err)
	}
	err := runWithApp([]string{"render-config"}, a, false)
	if err == nil || !strings.Contains(err.Error(), "is a directory") {
		t.Fatalf("expected runtime config write failure, got %v", err)
	}
}

func TestRunWithAppDispatchesStart(t *testing.T) {
	a, _ := newCLIApp(t)
	var calls []recordedCommand
	a.Runner = system.CommandRunner(recordingRunner{calls: &calls})
	mustImportNode(t, a, "trojan://password@example.org:443?security=tls#start-dispatch")
	if err := a.SetNodeEnabled(1, true); err != nil {
		t.Fatalf("enable node: %v", err)
	}
	err := runWithApp([]string{"start"}, a, false)
	if os.Geteuid() != 0 {
		if err == nil || !strings.Contains(err.Error(), "请用 root 运行") {
			t.Fatalf("expected root error, got %v", err)
		}
		return
	}
	if err != nil {
		t.Fatalf("run start: %v", err)
	}
	if !hasRecordedRunnerCall(calls, "systemctl", "enable", "--now", "minimalist.service") {
		t.Fatalf("expected systemctl enable --now call, got %#v", calls)
	}
}

func TestRunWithAppDispatchesStartPropagatesManifestError(t *testing.T) {
	a, _ := newCLIApp(t)
	var calls []recordedCommand
	a.Runner = system.CommandRunner(recordingRunner{calls: &calls})
	mustImportNode(t, a, "trojan://password@example.org:443?security=tls#start-manifest-failure")
	if err := a.SetNodeEnabled(1, true); err != nil {
		t.Fatalf("enable node: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(a.Paths.RulesRepoPath()), 0o755); err != nil {
		t.Fatalf("mkdir rules repo dir: %v", err)
	}
	if err := os.WriteFile(a.Paths.RulesRepoPath(), []byte("rulesets: [\n"), 0o640); err != nil {
		t.Fatalf("write invalid manifest: %v", err)
	}
	err := runWithApp([]string{"start"}, a, false)
	if !expectRootOrSpecificError(t, err, "parse manifest") {
		return
	}
	assertOnlyCutoverPreflightRunnerCalls(t, calls)
}

func TestRunWithAppDispatchesStartPropagatesManualProviderFailure(t *testing.T) {
	a, _ := newCLIApp(t)
	var calls []recordedCommand
	a.Runner = system.CommandRunner(recordingRunner{calls: &calls})
	mustImportNode(t, a, "trojan://password@example.org:443?security=tls#start-provider-failure")
	if err := a.SetNodeEnabled(1, true); err != nil {
		t.Fatalf("enable node: %v", err)
	}
	if err := os.MkdirAll(a.Paths.ManualProvider(), 0o755); err != nil {
		t.Fatalf("mkdir blocking manual provider path: %v", err)
	}
	err := runWithApp([]string{"start"}, a, false)
	if !expectRootOrSpecificError(t, err, "is a directory") {
		return
	}
	assertOnlyCutoverPreflightRunnerCalls(t, calls)
}

func TestRunWithAppDispatchesStartPropagatesRuntimeConfigFailure(t *testing.T) {
	a, _ := newCLIApp(t)
	var calls []recordedCommand
	a.Runner = system.CommandRunner(recordingRunner{calls: &calls})
	mustImportNode(t, a, "trojan://password@example.org:443?security=tls#start-runtime-config-failure")
	if err := a.SetNodeEnabled(1, true); err != nil {
		t.Fatalf("enable node: %v", err)
	}
	if err := os.MkdirAll(a.Paths.RuntimeConfig(), 0o755); err != nil {
		t.Fatalf("mkdir blocking runtime config path: %v", err)
	}
	err := runWithApp([]string{"start"}, a, false)
	if !expectRootOrSpecificError(t, err, "is a directory") {
		return
	}
	assertOnlyCutoverPreflightRunnerCalls(t, calls)
}

func TestRunWithAppDispatchesStop(t *testing.T) {
	a, _ := newCLIApp(t)
	var calls []recordedCommand
	a.Runner = system.CommandRunner(recordingRunner{calls: &calls})
	err := runWithApp([]string{"stop"}, a, false)
	if os.Geteuid() != 0 {
		if err == nil || !strings.Contains(err.Error(), "请用 root 运行") {
			t.Fatalf("expected root error, got %v", err)
		}
		return
	}
	if err != nil {
		t.Fatalf("run stop: %v", err)
	}
	if !hasRecordedRunnerCall(calls, "systemctl", "stop", "minimalist.service") {
		t.Fatalf("expected systemctl stop call, got %#v", calls)
	}
}

func TestRunWithAppDispatchesRestart(t *testing.T) {
	a, _ := newCLIApp(t)
	var calls []recordedCommand
	a.Runner = system.CommandRunner(recordingRunner{calls: &calls})
	mustImportNode(t, a, "trojan://password@example.org:443?security=tls#restart-dispatch")
	if err := a.SetNodeEnabled(1, true); err != nil {
		t.Fatalf("enable node: %v", err)
	}
	err := runWithApp([]string{"restart"}, a, false)
	if os.Geteuid() != 0 {
		if err == nil || !strings.Contains(err.Error(), "请用 root 运行") {
			t.Fatalf("expected root error, got %v", err)
		}
		return
	}
	if err != nil {
		t.Fatalf("run restart: %v", err)
	}
	if !hasRecordedRunnerCall(calls, "systemctl", "restart", "minimalist.service") {
		t.Fatalf("expected systemctl restart call, got %#v", calls)
	}
}

func TestRunWithAppDispatchesRestartPropagatesManifestError(t *testing.T) {
	a, _ := newCLIApp(t)
	var calls []recordedCommand
	a.Runner = system.CommandRunner(recordingRunner{calls: &calls})
	if err := os.MkdirAll(filepath.Dir(a.Paths.RulesRepoPath()), 0o755); err != nil {
		t.Fatalf("mkdir rules repo dir: %v", err)
	}
	if err := os.WriteFile(a.Paths.RulesRepoPath(), []byte("rulesets: [\n"), 0o640); err != nil {
		t.Fatalf("write invalid manifest: %v", err)
	}
	err := runWithApp([]string{"restart"}, a, false)
	if !expectRootOrSpecificError(t, err, "parse manifest") {
		return
	}
	assertOnlyCutoverPreflightRunnerCalls(t, calls)
}

func TestRunWithAppDispatchesRestartPropagatesCustomRulesFailure(t *testing.T) {
	a, _ := newCLIApp(t)
	var calls []recordedCommand
	a.Runner = system.CommandRunner(recordingRunner{calls: &calls})
	if err := os.MkdirAll(a.Paths.CustomRules(), 0o755); err != nil {
		t.Fatalf("mkdir blocking custom rules path: %v", err)
	}
	err := runWithApp([]string{"restart"}, a, false)
	if !expectRootOrSpecificError(t, err, "is a directory") {
		return
	}
	assertOnlyCutoverPreflightRunnerCalls(t, calls)
}

func TestRunWithAppDispatchesRestartPropagatesRuntimeConfigFailure(t *testing.T) {
	a, _ := newCLIApp(t)
	var calls []recordedCommand
	a.Runner = system.CommandRunner(recordingRunner{calls: &calls})
	if err := os.MkdirAll(a.Paths.RuntimeConfig(), 0o755); err != nil {
		t.Fatalf("mkdir blocking runtime config path: %v", err)
	}
	err := runWithApp([]string{"restart"}, a, false)
	if !expectRootOrSpecificError(t, err, "is a directory") {
		return
	}
	assertOnlyCutoverPreflightRunnerCalls(t, calls)
}

func TestRunWithAppDispatchesRouterWizard(t *testing.T) {
	a, stdout := newCLIApp(t)
	a.Stdin = strings.NewReader(strings.Repeat("\n", 14))
	if err := runWithApp([]string{"router-wizard"}, a, false); err != nil {
		t.Fatalf("run router-wizard: %v", err)
	}
	body, err := os.ReadFile(a.Paths.ConfigPath())
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !strings.Contains(string(body), "template: nas-single-lan-v4") {
		t.Fatalf("unexpected config after router-wizard:\n%s", string(body))
	}
	if !strings.Contains(stdout.String(), "旁路由参数已更新") {
		t.Fatalf("unexpected router-wizard output:\n%s", stdout.String())
	}
}

func TestRunWithAppDispatchesClearRules(t *testing.T) {
	a, _ := newCLIApp(t)
	a.Runner = system.CommandRunner(clearRulesSafeRunner{})
	err := runWithApp([]string{"clear-rules"}, a, false)
	if os.Geteuid() != 0 {
		if err == nil || !strings.Contains(err.Error(), "请用 root 运行") {
			t.Fatalf("expected root error, got %v", err)
		}
		return
	}
	if err != nil {
		t.Fatalf("run clear-rules: %v", err)
	}
}

func TestRunWithAppDispatchesApplyRules(t *testing.T) {
	a, stdout := newCLIApp(t)
	var calls []recordedCommand
	a.Runner = system.CommandRunner(applyRulesRecordingRunner{calls: &calls})
	mustImportNode(t, a, "trojan://password@example.org:443?security=tls#apply-dispatch")
	if err := a.SetNodeEnabled(1, true); err != nil {
		t.Fatalf("enable node: %v", err)
	}
	err := runWithApp([]string{"apply-rules"}, a, false)
	if os.Geteuid() != 0 {
		if err == nil || !strings.Contains(err.Error(), "请用 root 运行") {
			t.Fatalf("expected root error, got %v", err)
		}
		return
	}
	if err != nil {
		t.Fatalf("run apply-rules: %v", err)
	}
	for _, want := range []struct {
		name string
		args []string
	}{
		{"ip", []string{"route", "replace", "local", "table", "233"}},
		{"ip", []string{"rule", "add", "fwmark", "9011", "table", "233", "priority", "100"}},
	} {
		if !hasRecordedRunnerCall(calls, want.name, want.args...) {
			t.Fatalf("expected %s call with %v, got %#v", want.name, want.args, calls)
		}
	}
	if !strings.Contains(stdout.String(), "已应用路由规则") {
		t.Fatalf("unexpected apply-rules output:\n%s", stdout.String())
	}
}

func TestToggleNodeAndSubscriptionHelpersRequireIndices(t *testing.T) {
	a, _ := newCLIApp(t)
	if err := toggleNode(a, nil, true); err == nil || !strings.Contains(err.Error(), "usage: minimalist nodes enable|disable <index>") {
		t.Fatalf("expected toggleNode usage error, got %v", err)
	}
	if err := toggleSubscription(a, nil, true); err == nil || !strings.Contains(err.Error(), "usage: minimalist subscriptions enable|disable <index>") {
		t.Fatalf("expected toggleSubscription usage error, got %v", err)
	}
}

func TestRunRulesRepoFindJoinsMultiwordKeyword(t *testing.T) {
	a, _ := newCLIApp(t)
	err := runRulesRepo(a, []string{"find", "google", "dns"})
	if err != nil {
		t.Fatalf("run rules-repo find: %v", err)
	}
	if !strings.Contains(a.Stdout.(*bytes.Buffer).String(), "keyword=google dns") {
		t.Fatalf("expected joined keyword in output:\n%s", a.Stdout.(*bytes.Buffer).String())
	}
}

func TestRunRulesAndSubscriptionsHelperUsageErrors(t *testing.T) {
	a, _ := newCLIApp(t)
	for _, tc := range []struct {
		name string
		fn   func() error
		want string
	}{
		{"runRules", func() error { return runRules(a, false, nil) }, "usage: minimalist rules list|add|remove ..."},
		{"runRulesACL", func() error { return runRules(a, true, nil) }, "usage: minimalist acl list|add|remove ..."},
		{"runSubscriptions", func() error { return runSubscriptions(a, nil) }, "usage: minimalist subscriptions list|add|enable|disable|remove|update ..."},
	} {
		err := tc.fn()
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%s expected %q, got %v", tc.name, tc.want, err)
		}
	}
}
