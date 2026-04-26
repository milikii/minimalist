package cli

import (
	"bytes"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"minimalist/internal/app"
	"minimalist/internal/runtime"
	"minimalist/internal/system"
)

type noopRunner struct{}

func (noopRunner) Run(name string, args ...string) error { return nil }

func (noopRunner) Output(name string, args ...string) (string, string, error) {
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
