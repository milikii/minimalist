package app

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"minimalist/internal/config"
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
