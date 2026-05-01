package app

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	"minimalist/internal/config"
)

func TestHostProxyStatusReportsOffByDefault(t *testing.T) {
	app, _ := newTestApp(t)
	if err := app.HostProxyStatus(); err != nil {
		t.Fatalf("host proxy status: %v", err)
	}
	if got := app.Stdout.(*bytes.Buffer).String(); !strings.Contains(got, "off") {
		t.Fatalf("expected off status, got %q", got)
	}
}

func TestHostProxyStatusReportsOnWhenConfigured(t *testing.T) {
	app, _ := newTestApp(t)
	cfg, err := config.Ensure(app.Paths.ConfigPath())
	if err != nil {
		t.Fatalf("ensure config: %v", err)
	}
	cfg.Network.ProxyHostOutput = true
	if err := config.Save(app.Paths.ConfigPath(), cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	if err := app.HostProxyStatus(); err != nil {
		t.Fatalf("host proxy status: %v", err)
	}
	if got := app.Stdout.(*bytes.Buffer).String(); !strings.Contains(got, "on") {
		t.Fatalf("expected on status, got %q", got)
	}
}

func TestHostProxyEnableRejectsWhenNoEnabledManualNodes(t *testing.T) {
	app, _ := newTestApp(t)
	oldGeteuid := geteuid
	geteuid = func() int { return 0 }
	defer func() { geteuid = oldGeteuid }()
	app.Stdin = strings.NewReader("y\n")

	err := app.HostProxyEnable()
	if err == nil || !strings.Contains(err.Error(), "没有启用的手动节点") {
		t.Fatalf("expected enabled manual node error, got %v", err)
	}
}

func TestHostProxyEnableRollsBackConfigWhenApplyRulesFails(t *testing.T) {
	app := newTestAppWithEnabledManualNode(t)
	oldGeteuid := geteuid
	geteuid = func() int { return 0 }
	defer func() { geteuid = oldGeteuid }()
	app.Stdin = strings.NewReader("y\n")
	iptablesFailures := 0
	app.Runner = fakeRunner{
		runFn: func(name string, args ...string) error {
			if name == "iptables" {
				for _, arg := range args {
					if arg == "-C" {
						return errors.New("missing")
					}
				}
			}
			if name == "iptables" && iptablesFailures == 0 {
				for _, arg := range args {
					if arg == "-A" {
						iptablesFailures++
						return errors.New("iptables failed")
					}
				}
			}
			if name == "systemctl" {
				return errors.New("inactive")
			}
			return nil
		},
		outputFn: func(name string, args ...string) (string, string, error) {
			if name == "journalctl" {
				return "", "", nil
			}
			if name == "ip" && len(args) >= 3 && args[0] == "-4" && args[1] == "rule" && args[2] == "show" {
				return "", "", nil
			}
			return "", "", errors.New("unavailable")
		},
	}

	err := app.HostProxyEnable()
	if err == nil || !strings.Contains(err.Error(), "问题: 宿主机流量接管变更失败，但已恢复旧配置") || !strings.Contains(err.Error(), "下一步: 重新执行 minimalist host-proxy status 确认状态") {
		t.Fatalf("expected rollback error, got %v", err)
	}
	cfg, loadErr := config.Load(app.Paths.ConfigPath())
	if loadErr != nil {
		t.Fatalf("load config: %v", loadErr)
	}
	if cfg.Network.ProxyHostOutput {
		t.Fatalf("expected host proxy config to roll back, got %+v", cfg.Network)
	}
}

func TestHostProxyEnableCancelsWithoutMutation(t *testing.T) {
	app := newTestAppWithEnabledManualNode(t)
	oldGeteuid := geteuid
	geteuid = func() int { return 0 }
	defer func() { geteuid = oldGeteuid }()
	app.Stdin = strings.NewReader("n\n")

	if err := app.HostProxyEnable(); err != nil {
		t.Fatalf("host proxy enable: %v", err)
	}
	cfg, err := config.Load(app.Paths.ConfigPath())
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Network.ProxyHostOutput {
		t.Fatalf("expected config to stay off after cancel")
	}
}

func TestHostProxyEnableDoesNotMutateConfigWhenCutoverBlocked(t *testing.T) {
	app := newTestAppWithEnabledManualNode(t)
	oldGeteuid := geteuid
	geteuid = func() int { return 0 }
	defer func() { geteuid = oldGeteuid }()
	app.Stdin = strings.NewReader("y\n")
	app.Runner = fakeRunner{
		runFn: func(name string, args ...string) error {
			if name == "systemctl" && len(args) >= 3 && args[0] == "is-active" && args[2] == "mihomo.service" {
				return nil
			}
			return errors.New("inactive")
		},
		outputFn: func(name string, args ...string) (string, string, error) {
			return "", "", errors.New("unavailable")
		},
	}

	err := app.HostProxyEnable()
	if err == nil || !strings.Contains(err.Error(), "问题: 宿主机流量接管变更失败") || !strings.Contains(err.Error(), "cutover blocked") || !strings.Contains(err.Error(), "下一步: 先执行 minimalist cutover-preflight") {
		t.Fatalf("expected cutover blocked error, got %v", err)
	}
	cfg, loadErr := config.Load(app.Paths.ConfigPath())
	if loadErr != nil {
		t.Fatalf("load config: %v", loadErr)
	}
	if cfg.Network.ProxyHostOutput {
		t.Fatalf("expected config to remain off when cutover is blocked")
	}
}

func TestHostProxyEnableRollsBackConfigWhenRenderConfigFails(t *testing.T) {
	app := newTestAppWithEnabledManualNode(t)
	oldGeteuid := geteuid
	geteuid = func() int { return 0 }
	defer func() { geteuid = oldGeteuid }()
	app.Stdin = strings.NewReader("y\n")

	if err := os.Remove(app.Paths.RuntimeConfig()); err != nil && !os.IsNotExist(err) {
		t.Fatalf("remove runtime config: %v", err)
	}
	if err := os.MkdirAll(app.Paths.RuntimeConfig(), 0o755); err != nil {
		t.Fatalf("mkdir blocking runtime config path: %v", err)
	}

	err := app.HostProxyEnable()
	if err == nil || !strings.Contains(err.Error(), "rollback failed") {
		t.Fatalf("expected rollback failure after render error, got %v", err)
	}
	cfg, loadErr := config.Load(app.Paths.ConfigPath())
	if loadErr != nil {
		t.Fatalf("load config: %v", loadErr)
	}
	if cfg.Network.ProxyHostOutput {
		t.Fatalf("expected config truth to roll back after render failure")
	}
}

func TestHostProxyStatusReturnsActionableError(t *testing.T) {
	app, _ := newTestApp(t)
	oldGeteuid := geteuid
	geteuid = func() int { return 0 }
	defer func() { geteuid = oldGeteuid }()
	if err := os.WriteFile(app.Paths.ConfigDir, []byte("blocked"), 0o640); err != nil {
		t.Fatalf("write blocking config dir: %v", err)
	}

	err := app.HostProxyStatus()
	if err == nil || !strings.Contains(err.Error(), "问题: 读取宿主机接管状态失败") || !strings.Contains(err.Error(), "文档: README.md") {
		t.Fatalf("expected actionable status error, got %v", err)
	}
}
