package app

import (
	"bufio"
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"minimalist/internal/config"
	"minimalist/internal/provider"
	"minimalist/internal/runtime"
	"minimalist/internal/state"
)

//go:embed webui_static/*
var webUIStatic embed.FS

const defaultWebUIAddr = "0.0.0.0:18080"

// WebUIOptions configures the built-in operator web UI server.
type WebUIOptions struct {
	Addr     string
	Token    string
	AllowLAN bool
}

type webUIServer struct {
	app   *App
	token string
	mu    sync.Mutex
}

type webAPIResponse struct {
	OK     bool   `json:"ok"`
	Data   any    `json:"data,omitempty"`
	Output string `json:"output,omitempty"`
	Error  string `json:"error,omitempty"`
}

// WebUI starts the built-in browser control surface.
func (a *App) WebUI(opts WebUIOptions) error {
	if opts.Addr == "" {
		opts.Addr = defaultWebUIAddr
	}
	cfg, _, err := a.ensureAll()
	if err != nil {
		return err
	}
	token := strings.TrimSpace(opts.Token)
	if token == "" {
		token = strings.TrimSpace(cfg.Controller.Secret)
	}
	if token == "" {
		return errors.New("webui token is empty")
	}
	if err := validateWebUIExposure(opts.Addr, token); err != nil {
		return err
	}

	listener, err := net.Listen(webUIListenNetwork(opts.Addr), opts.Addr)
	if err != nil {
		return err
	}
	defer listener.Close()

	fmt.Fprintf(a.Stdout, "webui: http://%s/\n", listener.Addr().String())
	fmt.Fprintf(a.Stdout, "token: %s\n", token)
	if webUIAddrIsLoopback(opts.Addr) {
		fmt.Fprintln(a.Stdout, "远程访问: ssh -L 18080:127.0.0.1:18080 user@nas-host")
	} else {
		fmt.Fprintln(a.Stdout, "LAN 访问已开放；请从 NAS 的 LAN IP 访问，并妥善保管 token")
	}
	server := &http.Server{Handler: newWebUIHandler(a, token)}
	if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func newWebUIHandler(app *App, token string) http.Handler {
	server := &webUIServer{app: app, token: token}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/overview", server.auth(server.handleOverview))
	mux.HandleFunc("/api/nodes", server.auth(server.handleNodes))
	mux.HandleFunc("/api/nodes/import", server.auth(server.handleImportNodes))
	mux.HandleFunc("/api/nodes/", server.auth(server.handleNodeAction))
	mux.HandleFunc("/api/rules", server.auth(server.handleRules))
	mux.HandleFunc("/api/rules/", server.auth(server.handleRuleAction))
	mux.HandleFunc("/api/subscriptions", server.auth(server.handleSubscriptions))
	mux.HandleFunc("/api/subscriptions/", server.auth(server.handleSubscriptionAction))
	mux.HandleFunc("/api/config", server.auth(server.handleConfig))
	mux.HandleFunc("/api/config/render", server.auth(server.handleRenderConfig))
	mux.HandleFunc("/api/service/", server.auth(server.handleServiceAction))
	mux.HandleFunc("/api/logs", server.auth(server.handleLogs))
	mux.HandleFunc("/api/core/upgrade", server.auth(server.handleCoreUpgrade))
	mux.HandleFunc("/", serveWebUIStatic)
	return mux
}

func serveWebUIStatic(w http.ResponseWriter, r *http.Request) {
	staticRoot, err := fs.Sub(webUIStatic, "webui_static")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/")
	if path == "" {
		body, err := fs.ReadFile(staticRoot, "index.html")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(body)
		return
	}
	http.FileServer(http.FS(staticRoot)).ServeHTTP(w, withCleanPath(r, path))
}

func withCleanPath(r *http.Request, path string) *http.Request {
	cloned := r.Clone(r.Context())
	cloned.URL.Path = "/" + strings.TrimLeft(path, "/")
	return cloned
}

func (s *webUIServer) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		got := r.Header.Get("X-Minimalist-Token")
		if got == "" {
			got = strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		}
		if got == "" {
			got = r.URL.Query().Get("token")
		}
		if subtleTokenMismatch(got, s.token) {
			writeJSON(w, http.StatusUnauthorized, webAPIResponse{OK: false, Error: "unauthorized"})
			return
		}
		next(w, r)
	}
}

func subtleTokenMismatch(got, want string) bool {
	if want == "" || len(got) != len(want) {
		return true
	}
	var diff byte
	for i := range got {
		diff |= got[i] ^ want[i]
	}
	return diff != 0
}

func (s *webUIServer) handleOverview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cfg, st, err := s.app.ensureAll()
	if err != nil {
		writeError(w, err)
		return
	}
	snapshot, err := s.app.statusSnapshot()
	if err != nil {
		writeError(w, err)
		return
	}
	subEnabled, subTotal, subReady := s.app.subscriptionCounts(st)
	cutover := s.app.cutoverPreflightStatus()
	data := map[string]any{
		"snapshot":       snapshot,
		"manual_nodes":   webManualNodeCounts(st),
		"subscriptions":  map[string]int{"enabled": subEnabled, "total": subTotal, "ready": subReady},
		"runtime_assets": map[string]any{"missing": runtime.MissingRuntimeAssets(s.app.Paths)},
		"cutover_ready":  cutover.Ready(),
		"config":         webConfigSummary(cfg),
	}
	writeJSON(w, http.StatusOK, webAPIResponse{OK: true, Data: data})
}

func (s *webUIServer) handleNodes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, st, err := s.app.ensureAll()
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, webAPIResponse{OK: true, Data: webNodes(st.Nodes)})
}

func (s *webUIServer) handleImportNodes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var body struct {
		Links string `json:"links"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, err)
		return
	}
	output, err := s.runLocked(strings.NewReader(body.Links), func(app *App) error {
		return app.importLinksWithReader(bufio.NewReader(strings.NewReader(body.Links)))
	})
	writeActionResponse(w, output, err)
}

func (s *webUIServer) handleNodeAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	index, action, ok := splitIndexedAction(r.URL.Path, "/api/nodes/")
	if !ok {
		http.NotFound(w, r)
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if action == "rename" {
		if err := decodeJSON(r, &body); err != nil {
			writeError(w, err)
			return
		}
	}
	output, err := s.runLocked(nil, func(app *App) error {
		switch action {
		case "enable":
			return app.SetNodeEnabled(index, true)
		case "disable":
			return app.SetNodeEnabled(index, false)
		case "rename":
			return app.RenameNode(index, body.Name)
		case "remove":
			return app.RemoveNode(index)
		case "test":
			return app.TestNode(index)
		default:
			return fmt.Errorf("unknown node action: %s", action)
		}
	})
	writeActionResponse(w, output, err)
}

func (s *webUIServer) handleRules(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch r.Method {
	case http.MethodGet:
		_, st, err := s.app.ensureAll()
		if err != nil {
			writeError(w, err)
			return
		}
		scope := ruleScope(r)
		rules := st.Rules
		if scope == "acl" {
			rules = st.ACL
		}
		writeJSON(w, http.StatusOK, webAPIResponse{OK: true, Data: webRules(scope, rules)})
	case http.MethodPost:
		var body struct {
			Scope   string `json:"scope"`
			Kind    string `json:"kind"`
			Pattern string `json:"pattern"`
			Target  string `json:"target"`
		}
		if err := decodeJSON(r, &body); err != nil {
			writeError(w, err)
			return
		}
		output, err := s.runUnlocked(nil, func(app *App) error {
			return app.AddRule(body.Scope == "acl", body.Kind, body.Pattern, body.Target)
		})
		writeActionResponse(w, output, err)
	default:
		methodNotAllowed(w)
	}
}

func (s *webUIServer) handleRuleAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	index, action, ok := splitIndexedAction(r.URL.Path, "/api/rules/")
	if !ok || action != "remove" {
		http.NotFound(w, r)
		return
	}
	output, err := s.runLocked(nil, func(app *App) error {
		return app.RemoveRule(ruleScope(r) == "acl", index)
	})
	writeActionResponse(w, output, err)
}

func (s *webUIServer) handleSubscriptions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.mu.Lock()
		defer s.mu.Unlock()
		_, st, err := s.app.ensureAll()
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, webAPIResponse{OK: true, Data: webSubscriptions(st.Subscriptions)})
	case http.MethodPost:
		var body struct {
			Name    string `json:"name"`
			URL     string `json:"url"`
			Enabled bool   `json:"enabled"`
		}
		if err := decodeJSON(r, &body); err != nil {
			writeError(w, err)
			return
		}
		output, err := s.runLocked(nil, func(app *App) error {
			return app.AddSubscription(body.Name, body.URL, body.Enabled)
		})
		writeActionResponse(w, output, err)
	default:
		methodNotAllowed(w)
	}
}

func (s *webUIServer) handleSubscriptionAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	index, action, ok := splitIndexedAction(r.URL.Path, "/api/subscriptions/")
	if !ok {
		http.NotFound(w, r)
		return
	}
	output, err := s.runLocked(nil, func(app *App) error {
		switch action {
		case "enable":
			return app.SetSubscriptionEnabled(index, true)
		case "disable":
			return app.SetSubscriptionEnabled(index, false)
		case "remove":
			return app.RemoveSubscription(index)
		case "update":
			return app.UpdateSubscriptions()
		default:
			return fmt.Errorf("unknown subscription action: %s", action)
		}
	})
	writeActionResponse(w, output, err)
}

func (s *webUIServer) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.mu.Lock()
		defer s.mu.Unlock()
		cfg, _, err := s.app.ensureAll()
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, webAPIResponse{OK: true, Data: webConfigSummary(cfg)})
	case http.MethodPost:
		var body webConfigUpdate
		if err := decodeJSON(r, &body); err != nil {
			writeError(w, err)
			return
		}
		output, err := s.updateConfig(body)
		writeActionResponse(w, output, err)
	default:
		methodNotAllowed(w)
	}
}

func (s *webUIServer) handleRenderConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	output, err := s.runLocked(nil, func(app *App) error {
		return app.RenderConfig()
	})
	writeActionResponse(w, output, err)
}

func (s *webUIServer) handleServiceAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	action := strings.TrimPrefix(r.URL.Path, "/api/service/")
	output, err := s.runLocked(nil, func(app *App) error {
		switch action {
		case "start":
			return app.Start()
		case "stop":
			return app.Stop()
		case "restart":
			return app.Restart()
		case "apply-rules":
			return app.ApplyRules()
		case "clear-rules":
			return app.ClearRules()
		default:
			return fmt.Errorf("unknown service action: %s", action)
		}
	})
	writeActionResponse(w, output, err)
}

func (s *webUIServer) handleLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	lines, _ := strconv.Atoi(r.URL.Query().Get("lines"))
	opts := LogOptions{
		Target: r.URL.Query().Get("target"),
		Lines:  lines,
		Errors: r.URL.Query().Get("errors") == "1",
		Since:  r.URL.Query().Get("since"),
	}
	output, err := s.runLocked(nil, func(app *App) error {
		return app.Logs(opts)
	})
	writeActionResponse(w, output, err)
}

func (s *webUIServer) handleCoreUpgrade(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	output, err := s.runLocked(nil, func(app *App) error {
		return app.CoreUpgradeAlpha()
	})
	writeActionResponse(w, output, err)
}

func (s *webUIServer) runLocked(stdin io.Reader, fn func(*App) error) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.runUnlocked(stdin, fn)
}

func (s *webUIServer) runUnlocked(stdin io.Reader, fn func(*App) error) (string, error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if stdin == nil {
		stdin = strings.NewReader("")
	}
	clone := &App{
		Paths:  s.app.Paths,
		Runner: s.app.Runner,
		Client: s.app.Client,
		Stdout: &stdout,
		Stderr: &stderr,
		Stdin:  stdin,
	}
	err := fn(clone)
	return strings.TrimSpace(strings.TrimSpace(stdout.String()) + "\n" + strings.TrimSpace(stderr.String())), err
}

type webConfigUpdate struct {
	ProxyHostOutput         *bool    `json:"proxy_host_output"`
	ControllerBindAddress   *string  `json:"controller_bind_address"`
	LANCIDRs                []string `json:"lan_cidrs"`
	LANAllowedCIDRs         []string `json:"lan_allowed_cidrs"`
	LANDisallowedCIDRs      []string `json:"lan_disallowed_cidrs"`
	CoreAMD64CPULevel       *string  `json:"core_amd64_cpu_level"`
	CORSAllowPrivateNetwork *bool    `json:"cors_allow_private_network"`
}

func (s *webUIServer) updateConfig(body webConfigUpdate) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var output bytes.Buffer
	cfg, _, err := s.app.ensureAll()
	if err != nil {
		return "", err
	}
	if body.ControllerBindAddress != nil {
		cfg.Controller.BindAddress = strings.TrimSpace(*body.ControllerBindAddress)
	}
	if body.LANCIDRs != nil {
		cfg.Network.LANCIDRs = cleanStringList(body.LANCIDRs)
	}
	if body.LANAllowedCIDRs != nil {
		cfg.Access.LANAllowedCIDRs = cleanStringList(body.LANAllowedCIDRs)
	}
	if body.LANDisallowedCIDRs != nil {
		cfg.Access.LANDisallowedCIDRs = cleanStringList(body.LANDisallowedCIDRs)
	}
	if body.CoreAMD64CPULevel != nil {
		cfg.Install.CoreAMD64CPULevel = strings.TrimSpace(*body.CoreAMD64CPULevel)
	}
	if body.CORSAllowPrivateNetwork != nil {
		cfg.Controller.CORSAllowPrivateNetwork = *body.CORSAllowPrivateNetwork
	}
	hostProxyChanged := body.ProxyHostOutput != nil && cfg.Network.ProxyHostOutput != *body.ProxyHostOutput
	if !hostProxyChanged {
		if err := config.Save(s.app.Paths.ConfigPath(), cfg); err != nil {
			return "", err
		}
		return "配置已保存。需要时执行重新渲染或重启服务。", nil
	}
	desiredHostProxy := *body.ProxyHostOutput
	cfg.Network.ProxyHostOutput = !desiredHostProxy
	if err := config.Save(s.app.Paths.ConfigPath(), cfg); err != nil {
		return "", err
	}
	actionOutput, err := s.runUnlocked(nil, func(app *App) error {
		return app.setHostProxyFromWeb(desiredHostProxy)
	})
	output.WriteString(actionOutput)
	return strings.TrimSpace(output.String()), err
}

func (a *App) setHostProxyFromWeb(enabled bool) error {
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
	if enabled && !a.hasEnabledManualNodes(st) {
		return errors.New("没有启用的手动节点")
	}
	if cfg.Network.ProxyHostOutput == enabled {
		return nil
	}
	previous := cfg
	cfg.Network.ProxyHostOutput = enabled
	if err := a.persistHostProxyConfig(cfg); err != nil {
		if rollbackErr := a.rollbackHostProxy(previous); rollbackErr != nil {
			return fmt.Errorf("%w; rollback failed: %v", err, rollbackErr)
		}
		return err
	}
	return nil
}

func webConfigSummary(cfg config.Config) map[string]any {
	return map[string]any{
		"profile_template":           cfg.Profile.Template,
		"profile_mode":               cfg.Profile.Mode,
		"rule_preset":                cfg.Profile.RulePreset,
		"lan_interfaces":             cfg.Network.LANInterfaces,
		"lan_cidrs":                  cfg.Network.LANCIDRs,
		"proxy_ingress_interfaces":   cfg.Network.ProxyIngressInterfaces,
		"dns_hijack_enabled":         cfg.Network.DNSHijackEnabled,
		"dns_hijack_interfaces":      cfg.Network.DNSHijackInterfaces,
		"proxy_host_output":          cfg.Network.ProxyHostOutput,
		"lan_allowed_cidrs":          cfg.Access.LANAllowedCIDRs,
		"lan_disallowed_cidrs":       cfg.Access.LANDisallowedCIDRs,
		"controller_bind_address":    cfg.Controller.BindAddress,
		"controller_port":            cfg.Ports.Controller,
		"mixed_port":                 cfg.Ports.Mixed,
		"tproxy_port":                cfg.Ports.TProxy,
		"dns_port":                   cfg.Ports.DNS,
		"cors_allow_private_network": cfg.Controller.CORSAllowPrivateNetwork,
		"core_bin":                   cfg.Install.CoreBin,
		"core_amd64_cpu_level":       cfg.Install.CoreAMD64CPULevel,
	}
}

func webManualNodeCounts(st state.State) map[string]int {
	total := 0
	enabled := 0
	for _, node := range st.Nodes {
		if node.Source.Kind != "manual" {
			continue
		}
		total++
		if node.Enabled {
			enabled++
		}
	}
	return map[string]int{"total": total, "enabled": enabled}
}

func webNodes(nodes []state.Node) []map[string]any {
	out := make([]map[string]any, 0, len(nodes))
	for idx, node := range nodes {
		out = append(out, map[string]any{
			"index":       idx + 1,
			"id":          node.ID,
			"name":        node.Name,
			"enabled":     node.Enabled,
			"source":      node.Source.Kind,
			"source_id":   node.Source.ID,
			"imported_at": node.ImportedAt,
			"uri_preview": provider.URIBaseKey(node.URI),
		})
	}
	return out
}

func webRules(scope string, rules []state.Rule) []map[string]any {
	out := make([]map[string]any, 0, len(rules))
	for idx, rule := range rules {
		out = append(out, map[string]any{
			"index":   idx + 1,
			"id":      rule.ID,
			"scope":   scope,
			"kind":    rule.Kind,
			"pattern": rule.Pattern,
			"target":  rule.Target,
		})
	}
	return out
}

func webSubscriptions(subs []state.Subscription) []map[string]any {
	out := make([]map[string]any, 0, len(subs))
	for idx, sub := range subs {
		out = append(out, map[string]any{
			"index":           idx + 1,
			"id":              sub.ID,
			"name":            sub.Name,
			"enabled":         sub.Enabled,
			"created_at":      sub.CreatedAt,
			"last_success_at": sub.Cache.LastSuccessAt,
			"last_error":      sub.Cache.LastError,
			"last_count":      sub.Enumeration.LastCount,
		})
	}
	return out
}

func cleanStringList(values []string) []string {
	cleaned := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			cleaned = append(cleaned, value)
		}
	}
	return cleaned
}

func ruleScope(r *http.Request) string {
	if r.URL.Query().Get("scope") == "acl" {
		return "acl"
	}
	return "rules"
}

func splitIndexedAction(path, prefix string) (int, string, bool) {
	rest := strings.Trim(strings.TrimPrefix(path, prefix), "/")
	parts := strings.Split(rest, "/")
	if len(parts) != 2 {
		return 0, "", false
	}
	index, err := strconv.Atoi(parts[0])
	if err != nil || index <= 0 {
		return 0, "", false
	}
	return index, parts[1], true
}

func decodeJSON(r *http.Request, dst any) error {
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		return fmt.Errorf("decode request: %w", err)
	}
	return nil
}

func writeActionResponse(w http.ResponseWriter, output string, err error) {
	if err != nil {
		writeJSON(w, http.StatusBadRequest, webAPIResponse{OK: false, Output: output, Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, webAPIResponse{OK: true, Output: output})
}

func writeError(w http.ResponseWriter, err error) {
	writeJSON(w, http.StatusBadRequest, webAPIResponse{OK: false, Error: err.Error()})
}

func methodNotAllowed(w http.ResponseWriter) {
	writeJSON(w, http.StatusMethodNotAllowed, webAPIResponse{OK: false, Error: "method not allowed"})
}

func writeJSON(w http.ResponseWriter, status int, payload webAPIResponse) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func webUIAddrIsLoopback(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	host = strings.Trim(host, "[]")
	switch host {
	case "127.0.0.1", "localhost", "::1":
		return true
	case "":
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func webUITokenStrong(token string) bool {
	token = strings.TrimSpace(token)
	return len(token) >= 16 && token != "minimalist-secret"
}

func validateWebUIExposure(addr, token string) error {
	if webUIAddrIsLoopback(addr) {
		return nil
	}
	if !webUITokenStrong(token) {
		return errors.New("refusing to expose webui on LAN with weak token")
	}
	return nil
}

func webUIListenNetwork(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	host = strings.Trim(host, "[]")
	ip := net.ParseIP(host)
	if ip == nil {
		return "tcp"
	}
	if ip.To4() != nil {
		return "tcp4"
	}
	return "tcp6"
}
