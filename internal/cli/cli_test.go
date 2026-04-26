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
	"minimalist/internal/runtime"
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

func newCLIApp(t *testing.T) (*app.App, *bytes.Buffer) {
	t.Helper()
	root := t.TempDir()
	stdout := &bytes.Buffer{}
	return &app.App{
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
	}, stdout
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

func mustImportNode(t *testing.T, a *app.App, uri string) {
	t.Helper()
	a.Stdin = strings.NewReader(uri + "\n")
	if err := a.ImportLinks(); err != nil {
		t.Fatalf("import node: %v", err)
	}
}

func TestRunRulesRepoRequiresSubcommand(t *testing.T) {
	a, _ := newCLIApp(t)
	err := runRulesRepo(a, nil)
	if err == nil || !strings.Contains(err.Error(), "usage: minimalist rules-repo") {
		t.Fatalf("expected usage error, got %v", err)
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
		{nil, "usage: minimalist nodes list|rename|enable|disable|remove ..."},
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

func TestRunDispatchesRuntimeAuditThroughRun(t *testing.T) {
	setCLIPathsEnv(t)
	output := captureStdout(t, func() {
		if err := Run([]string{"runtime-audit"}); err != nil {
			t.Fatalf("run runtime-audit: %v", err)
		}
	})
	for _, needle := range []string{"alerts:", "providers-ready="} {
		if !strings.Contains(output, needle) {
			t.Fatalf("missing %q in runtime audit output:\n%s", needle, output)
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
