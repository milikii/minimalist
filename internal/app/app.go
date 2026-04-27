package app

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"minimalist/internal/config"
	"minimalist/internal/provider"
	"minimalist/internal/rulesrepo"
	"minimalist/internal/runtime"
	"minimalist/internal/state"
	"minimalist/internal/system"
)

type App struct {
	Paths  runtime.Paths
	Runner system.CommandRunner
	Client *http.Client
	Stdout io.Writer
	Stderr io.Writer
	Stdin  io.Reader
}

var legacyLiveInstall = struct {
	BinPath   string
	ConfigDir string
}{
	BinPath:   "/usr/local/bin/mihomo",
	ConfigDir: "/etc/mihomo",
}

type cutoverPreflightStatus struct {
	LegacyServiceActive      bool
	LegacyServiceEnabled     bool
	LegacyBin                bool
	LegacyConfigDir          bool
	MinimalistServiceActive  bool
	MinimalistServiceEnabled bool
	MinimalistUnit           bool
	MinimalistBin            bool
}

func (s cutoverPreflightStatus) legacyLive() bool {
	return s.LegacyServiceActive || s.LegacyServiceEnabled
}

func (s cutoverPreflightStatus) minimalistServiceLive() bool {
	return s.MinimalistServiceActive || s.MinimalistServiceEnabled
}

func (s cutoverPreflightStatus) Ready() bool {
	return !s.legacyLive() || s.minimalistServiceLive()
}

func New() *App {
	return &App{
		Paths:  runtime.DefaultPaths(),
		Runner: system.NewRunner(),
		Client: &http.Client{Timeout: 30 * time.Second},
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		Stdin:  os.Stdin,
	}
}

func (a *App) InstallSelf() error {
	src, err := os.Executable()
	if err != nil {
		return err
	}
	if err := a.requireRoot(); err != nil {
		return err
	}
	if err := runtime.EnsureLayout(a.Paths); err != nil {
		return err
	}
	in, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.WriteFile(a.Paths.BinPath, in, 0o755); err != nil {
		return err
	}
	if _, err := config.Ensure(a.Paths.ConfigPath()); err != nil {
		return err
	}
	if _, err := state.Ensure(a.Paths.StatePath()); err != nil {
		return err
	}
	if err := rulesrepo.InitDefaultRepo(filepath.Dir(a.Paths.RulesRepoPath())); err != nil {
		return err
	}
	fmt.Fprintf(a.Stdout, "已安装 minimalist 到 %s\n", a.Paths.BinPath)
	return nil
}

func (a *App) Setup() error {
	if err := a.requireRoot(); err != nil {
		return err
	}
	if err := a.ensureCutoverReady(); err != nil {
		return err
	}
	cfg, st, err := a.ensureAll()
	if err != nil {
		return err
	}
	if err := runtime.RenderFiles(a.Paths, cfg, st); err != nil {
		return err
	}
	if err := os.WriteFile(a.Paths.ServiceUnit, []byte(runtime.BuildServiceUnit(a.Paths, cfg)), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(a.Paths.SysctlPath, []byte(runtime.BuildSysctl(cfg)), 0o644); err != nil {
		return err
	}
	if err := a.Runner.Run("sysctl", "-p", a.Paths.SysctlPath); err != nil {
		return err
	}
	if err := a.Runner.Run("systemctl", "daemon-reload"); err != nil {
		return err
	}
	hasProviders := a.hasReadyProviders(st)
	if hasProviders {
		if err := a.Runner.Run("systemctl", "enable", "--now", "minimalist.service"); err != nil {
			return err
		}
		fmt.Fprintln(a.Stdout, "部署完成，服务已启用")
	} else {
		fmt.Fprintln(a.Stdout, "部署完成，请先 import-links 或 subscriptions update 后再启动服务")
	}
	return nil
}

func (a *App) RenderConfig() error {
	cfg, st, err := a.ensureAll()
	if err != nil {
		return err
	}
	if err := a.validateRuleTargets(st); err != nil {
		return err
	}
	if err := runtime.RenderFiles(a.Paths, cfg, st); err != nil {
		return err
	}
	fmt.Fprintf(a.Stdout, "已生成 %s\n", a.Paths.RuntimeConfig())
	return nil
}

func (a *App) Start() error {
	if err := a.requireRoot(); err != nil {
		return err
	}
	if err := a.ensureCutoverReady(); err != nil {
		return err
	}
	if err := a.RenderConfig(); err != nil {
		return err
	}
	return a.Runner.Run("systemctl", "enable", "--now", "minimalist.service")
}

func (a *App) Stop() error {
	if err := a.requireRoot(); err != nil {
		return err
	}
	return a.Runner.Run("systemctl", "stop", "minimalist.service")
}

func (a *App) Restart() error {
	if err := a.requireRoot(); err != nil {
		return err
	}
	if err := a.ensureCutoverReady(); err != nil {
		return err
	}
	if err := a.RenderConfig(); err != nil {
		return err
	}
	return a.Runner.Run("systemctl", "restart", "minimalist.service")
}

func (a *App) Status() error {
	cfg, st, err := a.ensureAll()
	if err != nil {
		return err
	}
	serviceActive := commandOK(a.Runner.Run("systemctl", "is-active", "--quiet", "minimalist.service"))
	serviceEnabled := commandOK(a.Runner.Run("systemctl", "is-enabled", "--quiet", "minimalist.service"))
	mode, source := a.currentMode(cfg)
	subEnabled, subTotal, subReady := a.subscriptionCounts(st)
	fmt.Fprintf(a.Stdout, "项目: minimalist\n")
	fmt.Fprintf(a.Stdout, "模板: %s\n", cfg.Profile.Template)
	fmt.Fprintf(a.Stdout, "规则预设: %s\n", cfg.Profile.RulePreset)
	fmt.Fprintf(a.Stdout, "当前模式: %s (%s)\n", mode, source)
	fmt.Fprintf(a.Stdout, "LAN 接口: %s\n", strings.Join(cfg.Network.LANInterfaces, " "))
	fmt.Fprintf(a.Stdout, "LAN 网段: %s\n", strings.Join(cfg.Network.LANCIDRs, " "))
	fmt.Fprintf(a.Stdout, "透明入口: %s\n", strings.Join(cfg.Network.ProxyIngressInterfaces, " "))
	fmt.Fprintf(a.Stdout, "宿主机接管: %t\n", cfg.Network.ProxyHostOutput)
	fmt.Fprintf(a.Stdout, "控制面: %s:%d\n", cfg.Controller.BindAddress, cfg.Ports.Controller)
	fmt.Fprintf(a.Stdout, "服务状态: active=%t enabled=%t\n", serviceActive, serviceEnabled)
	fmt.Fprintf(a.Stdout, "手动节点: %d\n", a.manualNodeCount(st))
	fmt.Fprintf(a.Stdout, "订阅: enabled=%d total=%d ready=%d\n", subEnabled, subTotal, subReady)
	return nil
}

func (a *App) ShowSecret() error {
	cfg, _, err := a.ensureAll()
	if err != nil {
		return err
	}
	fmt.Fprintln(a.Stdout, cfg.Controller.Secret)
	return nil
}

func (a *App) Healthcheck() error {
	cfg, _, err := a.ensureAll()
	if err != nil {
		return err
	}
	fmt.Fprintf(a.Stdout, "mixed-port=%d\n", cfg.Ports.Mixed)
	fmt.Fprintf(a.Stdout, "tproxy-port=%d\n", cfg.Ports.TProxy)
	fmt.Fprintf(a.Stdout, "dns-port=%d\n", cfg.Ports.DNS)
	fmt.Fprintf(a.Stdout, "controller-port=%d\n", cfg.Ports.Controller)
	if summary, err := a.controllerRuntimeSummary(cfg); err == nil {
		fmt.Fprintln(a.Stdout, summary)
	} else {
		fmt.Fprintf(a.Stdout, "controller: %v\n", err)
	}
	return nil
}

func (a *App) RuntimeAudit() error {
	cfg, st, err := a.ensureAll()
	if err != nil {
		return err
	}
	if err := a.Status(); err != nil {
		return err
	}
	stdout, _, _ := a.Runner.Output("journalctl", "-u", "minimalist.service", "--since", "24 hours ago", "--no-pager")
	warnCount := 0
	errCount := 0
	for _, line := range strings.Split(stdout, "\n") {
		lowered := strings.ToLower(line)
		if strings.Contains(lowered, "warn") {
			warnCount++
		}
		if strings.Contains(lowered, "error") {
			errCount++
		}
	}
	fmt.Fprintf(a.Stdout, "alerts: warn=%d error=%d\n", warnCount, errCount)
	fmt.Fprintf(a.Stdout, "providers-ready=%t\n", a.hasReadyProviders(st))
	a.printCutoverPreflight()
	if summary, err := a.controllerRuntimeSummary(cfg); err == nil {
		fmt.Fprintf(a.Stdout, "runtime: %s\n", summary)
	}
	return nil
}

func (a *App) CutoverPreflight() error {
	a.printCutoverPreflight()
	return nil
}

func (a *App) CutoverPlan() error {
	status := a.cutoverPreflightStatus()
	fmt.Fprintf(a.Stdout, "cutover-plan: legacy_live=%t minimalist_service_live=%t cutover_ready=%t\n", status.legacyLive(), status.minimalistServiceLive(), status.Ready())
	switch {
	case status.legacyLive() && !status.minimalistServiceLive():
		fmt.Fprintln(a.Stdout, "next-action: prepare-minimalist-inputs")
		fmt.Fprintln(a.Stdout, "maintenance-window: disable --now mihomo.service; rerun cutover-preflight; run setup and restart")
	case !status.legacyLive() && !status.minimalistServiceLive():
		fmt.Fprintln(a.Stdout, "next-action: run-minimalist-setup")
		fmt.Fprintln(a.Stdout, "maintenance-window: run setup, restart, healthcheck, status")
	default:
		fmt.Fprintln(a.Stdout, "next-action: validate-minimalist")
		fmt.Fprintln(a.Stdout, "maintenance-window: keep rollback path until healthcheck and routing state are stable")
	}
	fmt.Fprintln(a.Stdout, "rollback: disable --now minimalist.service; enable --now mihomo.service")
	return nil
}

func (a *App) printCutoverPreflight() {
	status := a.cutoverPreflightStatus()
	fmt.Fprintf(
		a.Stdout,
		"cutover-preflight: legacy_service_active=%t legacy_service_enabled=%t legacy_bin=%t legacy_config_dir=%t minimalist_service_active=%t minimalist_service_enabled=%t minimalist_unit=%t minimalist_bin=%t\n",
		status.LegacyServiceActive,
		status.LegacyServiceEnabled,
		status.LegacyBin,
		status.LegacyConfigDir,
		status.MinimalistServiceActive,
		status.MinimalistServiceEnabled,
		status.MinimalistUnit,
		status.MinimalistBin,
	)
	if status.legacyLive() && !status.minimalistServiceLive() {
		fmt.Fprintln(a.Stdout, "cutover-warning: legacy live install detected; do not run apply-rules or clear-rules before an explicit cutover plan")
	}
	if status.LegacyServiceActive || status.LegacyServiceEnabled || status.LegacyBin || status.LegacyConfigDir {
		fmt.Fprintln(a.Stdout, "cutover-note: legacy mihomo and minimalist use the same MIHOMO_* chains plus 0x2333/table 233 defaults")
	}
	fmt.Fprintf(a.Stdout, "cutover-ready=%t\n", status.Ready())
}

func (a *App) ensureCutoverReady() error {
	if !a.cutoverPreflightStatus().Ready() {
		return errors.New("cutover blocked: legacy mihomo.service is active or enabled; run minimalist cutover-preflight before high-risk commands")
	}
	return nil
}

func (a *App) cutoverPreflightStatus() cutoverPreflightStatus {
	return cutoverPreflightStatus{
		LegacyServiceActive:      commandOK(a.Runner.Run("systemctl", "is-active", "--quiet", "mihomo.service")),
		LegacyServiceEnabled:     commandOK(a.Runner.Run("systemctl", "is-enabled", "--quiet", "mihomo.service")),
		MinimalistServiceActive:  commandOK(a.Runner.Run("systemctl", "is-active", "--quiet", "minimalist.service")),
		MinimalistServiceEnabled: commandOK(a.Runner.Run("systemctl", "is-enabled", "--quiet", "minimalist.service")),
		LegacyBin:                pathExists(legacyLiveInstall.BinPath),
		LegacyConfigDir:          pathExists(legacyLiveInstall.ConfigDir),
		MinimalistUnit:           pathExists(a.Paths.ServiceUnit),
		MinimalistBin:            pathExists(a.Paths.BinPath),
	}
}

func (a *App) ImportLinks() error {
	_, st, err := a.ensureAll()
	if err != nil {
		return err
	}
	text, err := a.readImportInput()
	if err != nil {
		return err
	}
	rows := provider.ScanURIRows(text)
	supported := 0
	skipped := 0
	for _, row := range rows {
		if row.Supported == "1" {
			supported++
		} else {
			skipped++
		}
	}
	if supported == 0 {
		if skipped > 0 {
			fmt.Fprintf(a.Stderr, "有 %d 条链接因协议不受支持而被跳过\n", skipped)
		}
		return errors.New("没有读取到有效节点")
	}
	st.Nodes = provider.AppendImportedNodes(st.Nodes, rows, "manual", "", false)
	if err := state.Save(a.Paths.StatePath(), st); err != nil {
		return err
	}
	fmt.Fprintf(a.Stdout, "已处理 %d 条节点\n", supported)
	if skipped > 0 {
		fmt.Fprintf(a.Stdout, "有 %d 条链接因协议不受支持而被跳过\n", skipped)
	}
	return nil
}

func (a *App) RouterWizard() error {
	cfg, _, err := a.ensureAll()
	if err != nil {
		return err
	}
	reader := bufio.NewReader(a.Stdin)
	fmt.Fprintf(a.Stdout, "当前模板: %s\n", cfg.Profile.Template)
	cfg.Network.LANInterfaces = promptList(reader, a.Stdout, "LAN 接口", cfg.Network.LANInterfaces)
	cfg.Network.ProxyIngressInterfaces = promptList(reader, a.Stdout, "透明代理入口接口", cfg.Network.ProxyIngressInterfaces)
	cfg.Network.DNSHijackInterfaces = promptList(reader, a.Stdout, "DNS 劫持接口", cfg.Network.DNSHijackInterfaces)
	cfg.Network.ProxyHostOutput = promptBool(reader, a.Stdout, "宿主机流量接管", cfg.Network.ProxyHostOutput)
	cfg.Network.LANCIDRs = promptList(reader, a.Stdout, "LAN 网段", cfg.Network.LANCIDRs)
	cfg.Access.LANDisallowedCIDRs = promptList(reader, a.Stdout, "禁止访问网段", cfg.Access.LANDisallowedCIDRs)
	cfg.Access.Authentication = promptList(reader, a.Stdout, "显式代理认证", cfg.Access.Authentication)
	cfg.Access.SkipAuthPrefixes = promptList(reader, a.Stdout, "跳过认证前缀", cfg.Access.SkipAuthPrefixes)
	cfg.Controller.CORSAllowOrigins = promptList(reader, a.Stdout, "控制面 CORS origins", cfg.Controller.CORSAllowOrigins)
	cfg.Controller.CORSAllowPrivateNetwork = promptBool(reader, a.Stdout, "控制面允许 private network", cfg.Controller.CORSAllowPrivateNetwork)
	cfg.Network.Bypass.ContainerNames = promptList(reader, a.Stdout, "容器直连名单", cfg.Network.Bypass.ContainerNames)
	cfg.Network.Bypass.SrcCIDRs = promptList(reader, a.Stdout, "源地址直连名单", cfg.Network.Bypass.SrcCIDRs)
	cfg.Network.Bypass.DstCIDRs = promptList(reader, a.Stdout, "目标地址直连名单", cfg.Network.Bypass.DstCIDRs)
	cfg.Network.Bypass.UIDs = promptList(reader, a.Stdout, "UID 直连名单", cfg.Network.Bypass.UIDs)
	if err := config.Save(a.Paths.ConfigPath(), cfg); err != nil {
		return err
	}
	fmt.Fprintln(a.Stdout, "旁路由参数已更新")
	return nil
}

func (a *App) Menu() error {
	reader := bufio.NewReader(a.Stdin)
	for {
		fmt.Fprintln(a.Stdout, "1) 状态")
		fmt.Fprintln(a.Stdout, "2) 部署/修复")
		fmt.Fprintln(a.Stdout, "3) 导入节点")
		fmt.Fprintln(a.Stdout, "4) 订阅")
		fmt.Fprintln(a.Stdout, "5) 网络向导")
		fmt.Fprintln(a.Stdout, "6) 健康检查")
		fmt.Fprintln(a.Stdout, "7) 运行审计")
		fmt.Fprintln(a.Stdout, "8) 规则")
		fmt.Fprintln(a.Stdout, "9) ACL")
		fmt.Fprintln(a.Stdout, "0) 退出")
		fmt.Fprint(a.Stdout, "> ")
		line, _ := reader.ReadString('\n')
		switch strings.TrimSpace(line) {
		case "1":
			_ = a.Status()
		case "2":
			_ = a.Setup()
		case "3":
			_ = a.ImportLinks()
		case "4":
			_ = a.subscriptionsMenu(reader)
		case "5":
			_ = a.RouterWizard()
		case "6":
			_ = a.Healthcheck()
		case "7":
			_ = a.RuntimeAudit()
		case "8":
			_ = a.rulesMenu(reader, false)
		case "9":
			_ = a.rulesMenu(reader, true)
		case "0":
			return nil
		default:
			fmt.Fprintln(a.Stdout, "无效选择")
		}
	}
}

func (a *App) ListNodes() error {
	_, st, err := a.ensureAll()
	if err != nil {
		return err
	}
	for idx, node := range st.Nodes {
		fmt.Fprintf(a.Stdout, "%d\t%s\t%d\t%s\t%s\n", idx+1, node.Name, boolInt(node.Enabled), node.Source.Kind, node.URI)
	}
	return nil
}

func (a *App) RenameNode(index int, newName string) error {
	_, st, err := a.ensureAll()
	if err != nil {
		return err
	}
	node, err := a.nodeAt(&st, index)
	if err != nil {
		return err
	}
	if node.Source.Kind == "subscription" {
		return errors.New("subscription node is provider-managed")
	}
	oldName := node.Name
	node.Name = newName
	for idx := range st.Rules {
		if st.Rules[idx].Target == oldName {
			st.Rules[idx].Target = newName
		}
	}
	for idx := range st.ACL {
		if st.ACL[idx].Target == oldName {
			st.ACL[idx].Target = newName
		}
	}
	return state.Save(a.Paths.StatePath(), st)
}

func (a *App) SetNodeEnabled(index int, enabled bool) error {
	_, st, err := a.ensureAll()
	if err != nil {
		return err
	}
	node, err := a.nodeAt(&st, index)
	if err != nil {
		return err
	}
	if node.Source.Kind == "subscription" {
		return errors.New("subscription node is provider-managed")
	}
	node.Enabled = enabled
	return state.Save(a.Paths.StatePath(), st)
}

func (a *App) RemoveNode(index int) error {
	_, st, err := a.ensureAll()
	if err != nil {
		return err
	}
	if index < 1 || index > len(st.Nodes) {
		return errors.New("node index out of range")
	}
	st.Nodes = append(st.Nodes[:index-1], st.Nodes[index:]...)
	return state.Save(a.Paths.StatePath(), st)
}

func (a *App) ListRules(acl bool) error {
	_, st, err := a.ensureAll()
	if err != nil {
		return err
	}
	rules := st.Rules
	if acl {
		rules = st.ACL
	}
	for idx, rule := range rules {
		fmt.Fprintf(a.Stdout, "%d\t%s,%s,%s\n", idx+1, normalizeRuleKind(rule.Kind), rule.Pattern, rule.Target)
	}
	return nil
}

func (a *App) AddRule(acl bool, kind, pattern, target string) error {
	_, st, err := a.ensureAll()
	if err != nil {
		return err
	}
	rule := state.Rule{ID: newID(), Kind: normalizeRuleInput(kind), Pattern: pattern, Target: target}
	if err := a.validateTargetValue(st, rule.Target); err != nil {
		return err
	}
	if acl {
		st.ACL = appendIfMissingRule(st.ACL, rule)
	} else {
		st.Rules = appendIfMissingRule(st.Rules, rule)
	}
	return state.Save(a.Paths.StatePath(), st)
}

func (a *App) RemoveRule(acl bool, index int) error {
	_, st, err := a.ensureAll()
	if err != nil {
		return err
	}
	if acl {
		if index < 1 || index > len(st.ACL) {
			return errors.New("rule index out of range")
		}
		st.ACL = append(st.ACL[:index-1], st.ACL[index:]...)
	} else {
		if index < 1 || index > len(st.Rules) {
			return errors.New("rule index out of range")
		}
		st.Rules = append(st.Rules[:index-1], st.Rules[index:]...)
	}
	return state.Save(a.Paths.StatePath(), st)
}

func (a *App) ListSubscriptions() error {
	_, st, err := a.ensureAll()
	if err != nil {
		return err
	}
	for idx, sub := range st.Subscriptions {
		fmt.Fprintf(a.Stdout, "%d\t%s\t%s\t%d\t%s\t%d\t%s\n", idx+1, sub.Name, sub.URL, boolInt(sub.Enabled), sub.Cache.LastSuccessAt, sub.Enumeration.LastCount, sub.Cache.LastError)
	}
	return nil
}

func (a *App) AddSubscription(name, url string, enabled bool) error {
	_, st, err := a.ensureAll()
	if err != nil {
		return err
	}
	for idx := range st.Subscriptions {
		if st.Subscriptions[idx].URL == url {
			st.Subscriptions[idx].Name = name
			st.Subscriptions[idx].Enabled = enabled
			return state.Save(a.Paths.StatePath(), st)
		}
	}
	st.Subscriptions = append(st.Subscriptions, state.Subscription{
		ID:        newID(),
		Name:      name,
		URL:       url,
		Enabled:   enabled,
		CreatedAt: state.NowISO(),
		Cache: state.SubscriptionCache{
			LastAttemptAt: "",
			LastSuccessAt: "",
			LastError:     "",
		},
		Enumeration: state.SubscriptionEnumeration{
			LastCount:     0,
			LastUpdatedAt: "",
			Method:        "uri_scan",
		},
	})
	return state.Save(a.Paths.StatePath(), st)
}

func (a *App) SetSubscriptionEnabled(index int, enabled bool) error {
	_, st, err := a.ensureAll()
	if err != nil {
		return err
	}
	sub, err := subscriptionAt(&st, index)
	if err != nil {
		return err
	}
	sub.Enabled = enabled
	if !enabled {
		st.Nodes = purgeSubscriptionNodes(st.Nodes, sub.ID)
	}
	return state.Save(a.Paths.StatePath(), st)
}

func (a *App) RemoveSubscription(index int) error {
	_, st, err := a.ensureAll()
	if err != nil {
		return err
	}
	if index < 1 || index > len(st.Subscriptions) {
		return errors.New("subscription index out of range")
	}
	sub := st.Subscriptions[index-1]
	st.Subscriptions = append(st.Subscriptions[:index-1], st.Subscriptions[index:]...)
	st.Nodes = purgeSubscriptionNodes(st.Nodes, sub.ID)
	if err := os.Remove(a.Paths.SubscriptionFile(sub.ID)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return state.Save(a.Paths.StatePath(), st)
}

func (a *App) UpdateSubscriptions() error {
	_, st, err := a.ensureAll()
	if err != nil {
		return err
	}
	changed := false
	for idx := range st.Subscriptions {
		sub := &st.Subscriptions[idx]
		if !sub.Enabled {
			continue
		}
		if err := a.updateSubscription(sub, &st); err != nil {
			fmt.Fprintf(a.Stderr, "%s: %v\n", sub.Name, err)
		}
		changed = true
	}
	if changed {
		if err := state.Save(a.Paths.StatePath(), st); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) updateSubscription(sub *state.Subscription, st *state.State) error {
	sub.Cache.LastAttemptAt = state.NowISO()
	req, err := http.NewRequest(http.MethodGet, sub.URL, nil)
	if err != nil {
		sub.Cache.LastError = err.Error()
		return err
	}
	resp, err := a.httpClient().Do(req)
	if err != nil {
		sub.Cache.LastError = err.Error()
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		sub.Cache.LastError = fmt.Sprintf("http %d", resp.StatusCode)
		return fmt.Errorf("http %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		sub.Cache.LastError = err.Error()
		return err
	}
	if err := os.MkdirAll(a.Paths.SubscriptionDir(), 0o755); err != nil {
		sub.Cache.LastError = err.Error()
		return err
	}
	if err := os.WriteFile(a.Paths.SubscriptionFile(sub.ID), body, 0o640); err != nil {
		sub.Cache.LastError = err.Error()
		return err
	}
	rows := provider.ScanURIRows(string(body))
	sub.Cache.LastSuccessAt = state.NowISO()
	sub.Cache.LastError = ""
	sub.Enumeration.LastCount = 0
	sub.Enumeration.LastUpdatedAt = state.NowISO()
	sub.Enumeration.Method = "uri_scan"
	for _, row := range rows {
		if row.Supported == "1" {
			sub.Enumeration.LastCount++
		}
	}
	st.Nodes = purgeSubscriptionNodes(st.Nodes, sub.ID)
	st.Nodes = provider.AppendImportedNodes(st.Nodes, rows, "subscription", sub.ID, true)
	return nil
}

func (a *App) RulesRepoSummary() error {
	if _, _, err := a.ensureAll(); err != nil {
		return err
	}
	lines, err := rulesrepo.Describe(a.Paths.RulesRepoPath())
	if err != nil {
		return err
	}
	for _, line := range lines {
		fmt.Fprintln(a.Stdout, line)
	}
	return nil
}

func (a *App) RulesRepoEntries(name, keyword string) error {
	if _, _, err := a.ensureAll(); err != nil {
		return err
	}
	lines, err := rulesrepo.ListEntries(a.Paths.RulesRepoPath(), name, keyword)
	if err != nil {
		return err
	}
	for _, line := range lines {
		fmt.Fprintln(a.Stdout, line)
	}
	return nil
}

func (a *App) RulesRepoFind(keyword string) error {
	if _, _, err := a.ensureAll(); err != nil {
		return err
	}
	lines, err := rulesrepo.Search(a.Paths.RulesRepoPath(), keyword)
	if err != nil {
		return err
	}
	for _, line := range lines {
		fmt.Fprintln(a.Stdout, line)
	}
	return nil
}

func (a *App) RulesRepoAdd(name, value string) error {
	if _, _, err := a.ensureAll(); err != nil {
		return err
	}
	return rulesrepo.AppendEntry(a.Paths.RulesRepoPath(), name, value)
}

func (a *App) RulesRepoRemove(name, value string) error {
	if _, _, err := a.ensureAll(); err != nil {
		return err
	}
	return rulesrepo.RemoveEntry(a.Paths.RulesRepoPath(), name, value)
}

func (a *App) RulesRepoRemoveIndex(name string, index int) error {
	if _, _, err := a.ensureAll(); err != nil {
		return err
	}
	return rulesrepo.RemoveEntryIndex(a.Paths.RulesRepoPath(), name, index)
}

func (a *App) ApplyRules() error {
	if err := a.requireRoot(); err != nil {
		return err
	}
	if err := a.ensureCutoverReady(); err != nil {
		return err
	}
	cfg, st, err := a.ensureAll()
	if err != nil {
		return err
	}
	if len(cfg.Network.ProxyIngressInterfaces) == 0 && !cfg.Network.DNSHijackEnabled && !cfg.Network.ProxyHostOutput {
		if err := a.ClearRules(); err != nil {
			return err
		}
		fmt.Fprintln(a.Stdout, "当前模板为仅显式代理，不下发透明旁路由规则")
		return nil
	}
	if !a.hasEnabledManualNodes(st) {
		return errors.New("没有启用的手动节点")
	}
	if err := a.ClearRules(); err != nil {
		return err
	}
	reserved := []string{"0.0.0.0/8", "10.0.0.0/8", "127.0.0.0/8", "169.254.0.0/16", "172.16.0.0/12", "192.168.0.0/16", "224.0.0.0/4", "240.0.0.0/4"}
	if err := a.ensureChain("mangle", "MIHOMO_PRE"); err != nil {
		return err
	}
	if err := a.ensureChain("mangle", "MIHOMO_PRE_HANDLE"); err != nil {
		return err
	}
	if err := a.ensureChain("mangle", "MIHOMO_OUT"); err != nil {
		return err
	}
	if err := a.ensureChain("nat", "MIHOMO_DNS"); err != nil {
		return err
	}
	if err := a.ensureChain("nat", "MIHOMO_DNS_HANDLE"); err != nil {
		return err
	}
	for _, iface := range cfg.Network.ProxyIngressInterfaces {
		if err := a.ipt("mangle", "-A", "MIHOMO_PRE", "-i", iface, "-j", "MIHOMO_PRE_HANDLE"); err != nil {
			return err
		}
	}
	if err := a.ipt("mangle", "-A", "MIHOMO_PRE", "-j", "RETURN"); err != nil {
		return err
	}
	if cfg.Network.DNSHijackEnabled {
		if err := a.ipt("mangle", "-A", "MIHOMO_PRE_HANDLE", "-p", "udp", "--dport", "53", "-j", "RETURN"); err != nil {
			return err
		}
		if err := a.ipt("mangle", "-A", "MIHOMO_PRE_HANDLE", "-p", "tcp", "--dport", "53", "-j", "RETURN"); err != nil {
			return err
		}
	}
	for _, cidr := range append(append([]string{}, reserved...), cfg.Network.Bypass.DstCIDRs...) {
		if cidr == "" {
			continue
		}
		if err := a.ipt("mangle", "-A", "MIHOMO_PRE_HANDLE", "-d", cidr, "-j", "RETURN"); err != nil {
			return err
		}
		if err := a.ipt("mangle", "-A", "MIHOMO_OUT", "-d", cidr, "-j", "RETURN"); err != nil {
			return err
		}
	}
	for _, cidr := range cfg.Network.Bypass.SrcCIDRs {
		if cidr == "" {
			continue
		}
		if err := a.ipt("mangle", "-A", "MIHOMO_PRE_HANDLE", "-s", cidr, "-j", "RETURN"); err != nil {
			return err
		}
		if err := a.ipt("nat", "-A", "MIHOMO_DNS_HANDLE", "-s", cidr, "-j", "RETURN"); err != nil {
			return err
		}
	}
	for _, cidr := range a.containerBypassIPs(cfg.Network.Bypass.ContainerNames) {
		if err := a.ipt("mangle", "-A", "MIHOMO_PRE_HANDLE", "-s", cidr, "-j", "RETURN"); err != nil {
			return err
		}
		if err := a.ipt("nat", "-A", "MIHOMO_DNS_HANDLE", "-s", cidr, "-j", "RETURN"); err != nil {
			return err
		}
	}
	mark, mask, routeTable, priority := "0x2333", "0xffffffff", "233", "100"
	if err := a.ipt("mangle", "-A", "MIHOMO_PRE_HANDLE", "-p", "tcp", "-j", "TPROXY", "--on-port", strconv.Itoa(cfg.Ports.TProxy), "--tproxy-mark", mark+"/"+mask); err != nil {
		return err
	}
	if err := a.ipt("mangle", "-A", "MIHOMO_PRE_HANDLE", "-p", "udp", "-j", "TPROXY", "--on-port", strconv.Itoa(cfg.Ports.TProxy), "--tproxy-mark", mark+"/"+mask); err != nil {
		return err
	}
	if err := a.ipt("mangle", "-A", "MIHOMO_OUT", "-m", "owner", "--uid-owner", "root", "-j", "RETURN"); err != nil {
		return err
	}
	if err := a.ipt("mangle", "-A", "MIHOMO_OUT", "-m", "conntrack", "--ctdir", "REPLY", "-j", "RETURN"); err != nil {
		return err
	}
	for _, uid := range cfg.Network.Bypass.UIDs {
		if uid == "" {
			continue
		}
		if err := a.ipt("mangle", "-A", "MIHOMO_OUT", "-m", "owner", "--uid-owner", uid, "-j", "RETURN"); err != nil {
			return err
		}
	}
	if err := a.ipt("mangle", "-A", "MIHOMO_OUT", "-p", "tcp", "-j", "MARK", "--set-mark", "9011"); err != nil {
		return err
	}
	if err := a.ipt("mangle", "-A", "MIHOMO_OUT", "-p", "udp", "-j", "MARK", "--set-mark", "9011"); err != nil {
		return err
	}
	if cfg.Network.DNSHijackEnabled {
		for _, iface := range cfg.Network.DNSHijackInterfaces {
			if err := a.ipt("nat", "-A", "MIHOMO_DNS", "-i", iface, "-j", "MIHOMO_DNS_HANDLE"); err != nil {
				return err
			}
		}
		if err := a.ipt("nat", "-A", "MIHOMO_DNS", "-j", "RETURN"); err != nil {
			return err
		}
		if err := a.ipt("nat", "-A", "MIHOMO_DNS_HANDLE", "-p", "udp", "--dport", "53", "-j", "REDIRECT", "--to-ports", strconv.Itoa(cfg.Ports.DNS)); err != nil {
			return err
		}
		if err := a.ipt("nat", "-A", "MIHOMO_DNS_HANDLE", "-p", "tcp", "--dport", "53", "-j", "REDIRECT", "--to-ports", strconv.Itoa(cfg.Ports.DNS)); err != nil {
			return err
		}
		if !commandOK(a.iptCheck("nat", "PREROUTING", "-j", "MIHOMO_DNS")) {
			if err := a.ipt("nat", "-A", "PREROUTING", "-j", "MIHOMO_DNS"); err != nil {
				return err
			}
		}
	}
	if !commandOK(a.iptCheck("mangle", "PREROUTING", "-j", "MIHOMO_PRE")) {
		if err := a.ipt("mangle", "-A", "PREROUTING", "-j", "MIHOMO_PRE"); err != nil {
			return err
		}
	}
	if cfg.Network.ProxyHostOutput && !commandOK(a.iptCheck("mangle", "OUTPUT", "-j", "MIHOMO_OUT")) {
		if err := a.ipt("mangle", "-A", "OUTPUT", "-j", "MIHOMO_OUT"); err != nil {
			return err
		}
	}
	_ = a.deleteIPRule(routeTable, priority)
	if err := a.Runner.Run("ip", "-4", "route", "replace", "local", "0.0.0.0/0", "dev", "lo", "table", routeTable); err != nil {
		return err
	}
	if err := a.Runner.Run("ip", "-4", "rule", "add", "fwmark", "9011", "table", routeTable, "priority", priority); err != nil {
		return err
	}
	fmt.Fprintln(a.Stdout, "已应用路由规则")
	return nil
}

func (a *App) ClearRules() error {
	if err := a.requireRoot(); err != nil {
		return err
	}
	if err := a.ensureCutoverReady(); err != nil {
		return err
	}
	for _, item := range []struct {
		table string
		chain string
		rule  []string
	}{
		{"mangle", "PREROUTING", []string{"-j", "MIHOMO_PRE"}},
		{"mangle", "OUTPUT", []string{"-j", "MIHOMO_OUT"}},
		{"nat", "PREROUTING", []string{"-j", "MIHOMO_DNS"}},
		{"nat", "OUTPUT", []string{"-j", "MIHOMO_DNS_OUT"}},
	} {
		if err := a.deleteJump(item.table, item.chain, item.rule...); err != nil {
			return err
		}
	}
	for _, item := range []struct {
		table string
		chain string
	}{
		{"mangle", "MIHOMO_PRE"},
		{"mangle", "MIHOMO_PRE_HANDLE"},
		{"mangle", "MIHOMO_OUT"},
		{"nat", "MIHOMO_DNS"},
		{"nat", "MIHOMO_DNS_HANDLE"},
		{"nat", "MIHOMO_DNS_OUT"},
	} {
		_ = a.ipt(item.table, "-F", item.chain)
		_ = a.ipt(item.table, "-X", item.chain)
	}
	_ = a.deleteIPRule("233", "100")
	_ = a.Runner.Run("ip", "-4", "route", "flush", "table", "233")
	return nil
}

func (a *App) ensureAll() (config.Config, state.State, error) {
	if err := runtime.EnsureLayout(a.Paths); err != nil {
		return config.Config{}, state.State{}, err
	}
	cfg, err := config.Ensure(a.Paths.ConfigPath())
	if err != nil {
		return config.Config{}, state.State{}, err
	}
	st, err := state.Ensure(a.Paths.StatePath())
	if err != nil {
		return config.Config{}, state.State{}, err
	}
	if err := rulesrepo.InitDefaultRepo(filepath.Dir(a.Paths.RulesRepoPath())); err != nil {
		return config.Config{}, state.State{}, err
	}
	return cfg, st, nil
}

func (a *App) requireRoot() error {
	if geteuid() != 0 {
		return errors.New("请用 root 运行")
	}
	return nil
}

func (a *App) httpClient() *http.Client {
	if a.Client != nil {
		return a.Client
	}
	return &http.Client{Timeout: 30 * time.Second}
}

func (a *App) hasReadyProviders(st state.State) bool {
	if a.hasEnabledManualNodes(st) {
		return true
	}
	for _, sub := range st.Subscriptions {
		if !sub.Enabled {
			continue
		}
		if info, err := os.Stat(a.Paths.SubscriptionFile(sub.ID)); err == nil && info.Size() > 0 {
			return true
		}
	}
	return false
}

func (a *App) hasEnabledManualNodes(st state.State) bool {
	for _, node := range st.Nodes {
		if node.Enabled && node.Source.Kind != "subscription" {
			return true
		}
	}
	return false
}

func (a *App) manualNodeCount(st state.State) int {
	count := 0
	for _, node := range st.Nodes {
		if node.Source.Kind != "subscription" {
			count++
		}
	}
	return count
}

func (a *App) subscriptionCounts(st state.State) (enabled int, total int, ready int) {
	total = len(st.Subscriptions)
	for _, sub := range st.Subscriptions {
		if sub.Enabled {
			enabled++
			if info, err := os.Stat(a.Paths.SubscriptionFile(sub.ID)); err == nil && info.Size() > 0 {
				ready++
			}
		}
	}
	return
}

func (a *App) currentMode(cfg config.Config) (string, string) {
	if runtimeMode, err := a.controllerConfigMode(cfg); err == nil && runtimeMode != "" {
		return runtimeMode, "runtime"
	}
	return cfg.Profile.Mode, "config"
}

func (a *App) controllerRuntimeSummary(cfg config.Config) (string, error) {
	host := cfg.Controller.BindAddress
	if host == "0.0.0.0" || host == "*" || host == "" {
		host = "127.0.0.1"
	}
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://%s:%d/version", host, cfg.Ports.Controller), nil)
	if err != nil {
		return "", err
	}
	if cfg.Controller.Secret != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.Controller.Secret)
	}
	resp, err := a.httpClient().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(body)), nil
}

func (a *App) controllerConfigMode(cfg config.Config) (string, error) {
	host := cfg.Controller.BindAddress
	if host == "0.0.0.0" || host == "*" || host == "" {
		host = "127.0.0.1"
	}
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://%s:%d/configs", host, cfg.Ports.Controller), nil)
	if err != nil {
		return "", err
	}
	if cfg.Controller.Secret != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.Controller.Secret)
	}
	resp, err := a.httpClient().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	if mode, ok := payload["mode"].(string); ok {
		return mode, nil
	}
	return "", nil
}

func (a *App) readImportInput() (string, error) {
	if terminalCheck(a.Stdin) {
		fmt.Fprintln(a.Stdout, "请粘贴节点链接，输入 end 结束：")
	}
	reader := bufio.NewReader(a.Stdin)
	lines := []string{}
	for {
		line, err := reader.ReadString('\n')
		if errors.Is(err, io.EOF) {
			line = strings.TrimRight(line, "\r\n")
			if line != "" {
				lines = append(lines, line)
			}
			break
		}
		if err != nil {
			return "", err
		}
		line = strings.TrimRight(line, "\r\n")
		if terminalCheck(a.Stdin) && strings.EqualFold(line, "end") {
			break
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n"), nil
}

func (a *App) validateRuleTargets(st state.State) error {
	enabled := map[string]struct{}{}
	for _, node := range st.Nodes {
		if node.Enabled && node.Source.Kind != "subscription" {
			enabled[node.Name] = struct{}{}
		}
	}
	allowed := map[string]struct{}{"DIRECT": {}, "PROXY": {}, "REJECT": {}}
	if len(enabled) > 0 {
		allowed["AUTO"] = struct{}{}
	}
	for name := range enabled {
		allowed[name] = struct{}{}
	}
	for _, ruleSet := range [][]state.Rule{st.Rules, st.ACL} {
		for _, rule := range ruleSet {
			if _, ok := allowed[rule.Target]; !ok {
				return fmt.Errorf("无效规则目标: %s", rule.Target)
			}
		}
	}
	return nil
}

func (a *App) validateTargetValue(st state.State, target string) error {
	allowed := []string{"DIRECT", "PROXY", "REJECT", "AUTO"}
	if slices.Contains(allowed, target) {
		if target == "AUTO" && !a.hasEnabledManualNodes(st) {
			return errors.New("AUTO 需要至少一个启用的手动节点")
		}
		return nil
	}
	for _, node := range st.Nodes {
		if node.Name == target && node.Source.Kind != "subscription" {
			return nil
		}
	}
	return fmt.Errorf("未知规则目标: %s", target)
}

func (a *App) nodeAt(st *state.State, index int) (*state.Node, error) {
	if index < 1 || index > len(st.Nodes) {
		return nil, errors.New("node index out of range")
	}
	return &st.Nodes[index-1], nil
}

func (a *App) subscriptionsMenu(reader *bufio.Reader) error {
	fmt.Fprintln(a.Stdout, "1) 查看订阅")
	fmt.Fprintln(a.Stdout, "2) 更新订阅")
	fmt.Fprint(a.Stdout, "> ")
	line, _ := reader.ReadString('\n')
	switch strings.TrimSpace(line) {
	case "1":
		return a.ListSubscriptions()
	case "2":
		return a.UpdateSubscriptions()
	default:
		return nil
	}
}

func (a *App) rulesMenu(reader *bufio.Reader, acl bool) error {
	fmt.Fprintln(a.Stdout, "1) 查看")
	fmt.Fprintln(a.Stdout, "2) 添加")
	fmt.Fprintln(a.Stdout, "3) 删除")
	fmt.Fprint(a.Stdout, "> ")
	line, _ := reader.ReadString('\n')
	switch strings.TrimSpace(line) {
	case "1":
		return a.ListRules(acl)
	case "2":
		kind := promptString(reader, a.Stdout, "kind", "")
		pattern := promptString(reader, a.Stdout, "pattern", "")
		target := promptString(reader, a.Stdout, "target", "")
		return a.AddRule(acl, kind, pattern, target)
	case "3":
		index, _ := strconv.Atoi(promptString(reader, a.Stdout, "index", "1"))
		return a.RemoveRule(acl, index)
	default:
		return nil
	}
}

func (a *App) ensureChain(table, chain string) error {
	if commandOK(a.ipt(table, "-S", chain)) {
		return a.ipt(table, "-F", chain)
	}
	return a.ipt(table, "-N", chain)
}

func (a *App) deleteJump(table, chain string, rule ...string) error {
	for commandOK(a.iptCheck(table, chain, rule...)) {
		args := append([]string{"-D", chain}, rule...)
		if err := a.ipt(table, args...); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) deleteIPRule(routeTable, priority string) error {
	for {
		err := a.Runner.Run("ip", "-4", "rule", "del", "fwmark", "9011", "table", routeTable, "priority", priority)
		if err != nil {
			return nil
		}
	}
}

func (a *App) containerBypassIPs(names []string) []string {
	ips := []string{}
	for _, name := range names {
		stdout, _, err := a.Runner.Output("docker", "inspect", name, "--format", "{{range .NetworkSettings.Networks}}{{println .IPAddress}}{{end}}")
		if err != nil {
			continue
		}
		for _, line := range strings.Split(stdout, "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				ips = append(ips, line+"/32")
			}
		}
	}
	slices.Sort(ips)
	return slices.Compact(ips)
}

func (a *App) ipt(table string, args ...string) error {
	all := append([]string{"-w", "5", "-t", table}, args...)
	return a.Runner.Run("iptables", all...)
}

func (a *App) iptCheck(table, chain string, rule ...string) error {
	args := append([]string{"-w", "5", "-t", table, "-C", chain}, rule...)
	return a.Runner.Run("iptables", args...)
}

func commandOK(err error) bool { return err == nil }

func promptList(reader *bufio.Reader, out io.Writer, label string, current []string) []string {
	fmt.Fprintf(out, "%s [%s]: ", label, strings.Join(current, " "))
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		return current
	}
	return splitFields(line)
}

func promptString(reader *bufio.Reader, out io.Writer, label, current string) string {
	fmt.Fprintf(out, "%s [%s]: ", label, current)
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		return current
	}
	return line
}

func promptBool(reader *bufio.Reader, out io.Writer, label string, current bool) bool {
	currentValue := "0"
	if current {
		currentValue = "1"
	}
	fmt.Fprintf(out, "%s [0/1][%s]: ", label, currentValue)
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		return current
	}
	return line == "1"
}

func splitFields(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return strings.Fields(value)
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func normalizeRuleInput(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "domain":
		return "domain"
	case "domain-suffix", "suffix", "domain_suffix":
		return "suffix"
	case "domain-keyword", "keyword", "domain_keyword":
		return "keyword"
	case "src-ip-cidr", "src-cidr", "source", "src":
		return "src-cidr"
	case "ip", "ip-cidr", "dst-cidr", "dst":
		return "ip-cidr"
	case "port", "dst-port":
		return "port"
	case "geoip":
		return "geoip"
	case "geosite":
		return "geosite"
	case "ruleset", "rule-set":
		return "ruleset"
	default:
		return strings.ToLower(strings.TrimSpace(kind))
	}
}

func normalizeRuleKind(kind string) string {
	switch kind {
	case "domain":
		return "DOMAIN"
	case "suffix":
		return "DOMAIN-SUFFIX"
	case "keyword":
		return "DOMAIN-KEYWORD"
	case "src-cidr":
		return "SRC-IP-CIDR"
	case "ip-cidr":
		return "IP-CIDR"
	case "port":
		return "DST-PORT"
	case "geoip":
		return "GEOIP"
	case "geosite":
		return "GEOSITE"
	case "ruleset":
		return "RULE-SET"
	default:
		return strings.ToUpper(kind)
	}
}

func appendIfMissingRule(rules []state.Rule, candidate state.Rule) []state.Rule {
	for _, item := range rules {
		if item.Kind == candidate.Kind && item.Pattern == candidate.Pattern && item.Target == candidate.Target {
			return rules
		}
	}
	return append(rules, candidate)
}

func subscriptionAt(st *state.State, index int) (*state.Subscription, error) {
	if index < 1 || index > len(st.Subscriptions) {
		return nil, errors.New("subscription index out of range")
	}
	return &st.Subscriptions[index-1], nil
}

func purgeSubscriptionNodes(nodes []state.Node, subscriptionID string) []state.Node {
	filtered := nodes[:0]
	for _, node := range nodes {
		if node.Source.Kind == "subscription" && node.Source.ID == subscriptionID {
			continue
		}
		filtered = append(filtered, node)
	}
	return filtered
}

func newID() string {
	return strconv.FormatInt(time.Now().UnixNano(), 36)
}

func isTerminal(r io.Reader) bool {
	f, ok := r.(*os.File)
	if !ok {
		return false
	}
	_, err := f.Stat()
	if err != nil {
		return false
	}
	return (syscall.Stdin >= 0) && isCharDevice(f)
}

func isCharDevice(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

var terminalCheck = isTerminal
var geteuid = os.Geteuid
