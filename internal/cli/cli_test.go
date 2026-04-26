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
