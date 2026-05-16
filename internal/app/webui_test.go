package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"minimalist/internal/config"
	"minimalist/internal/state"
)

func TestWebUIStaticIsPublicAndAPIRequiresToken(t *testing.T) {
	app, _ := newTestApp(t)
	handler := newWebUIHandler(app, "test-token-123456")

	indexReq := httptest.NewRequest(http.MethodGet, "/", nil)
	indexResp := httptest.NewRecorder()
	handler.ServeHTTP(indexResp, indexReq)
	if indexResp.Code != http.StatusOK || !strings.Contains(indexResp.Body.String(), "minimalist 控制面") {
		t.Fatalf("expected index page, code=%d body=%s", indexResp.Code, indexResp.Body.String())
	}

	apiReq := httptest.NewRequest(http.MethodGet, "/api/overview", nil)
	apiResp := httptest.NewRecorder()
	handler.ServeHTTP(apiResp, apiReq)
	if apiResp.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized API response, got %d", apiResp.Code)
	}
}

func TestWebUIOverviewReturnsOperatorState(t *testing.T) {
	app := newTestAppWithEnabledManualNode(t)
	handler := newWebUIHandler(app, "test-token-123456")

	resp := authedRequest(t, handler, http.MethodGet, "/api/overview", "", "test-token-123456")
	if resp.Code != http.StatusOK {
		t.Fatalf("overview failed: %d %s", resp.Code, resp.Body.String())
	}
	body := decodeWebResponse(t, resp)
	data := body["data"].(map[string]any)
	snapshot := data["snapshot"].(map[string]any)
	if snapshot["NodeState"] != "ready" {
		t.Fatalf("unexpected snapshot: %#v", snapshot)
	}
	configSummary := data["config"].(map[string]any)
	if configSummary["controller_bind_address"] != "127.0.0.1" {
		t.Fatalf("unexpected config summary: %#v", configSummary)
	}
}

func TestWebUINodeImportAndEnableUseAppState(t *testing.T) {
	app, _ := newTestApp(t)
	handler := newWebUIHandler(app, "test-token-123456")
	body := `{"links":"trojan://password@example.org:443?security=tls#web-node"}`
	resp := authedRequest(t, handler, http.MethodPost, "/api/nodes/import", body, "test-token-123456")
	if resp.Code != http.StatusOK {
		t.Fatalf("import failed: %d %s", resp.Code, resp.Body.String())
	}

	resp = authedRequest(t, handler, http.MethodPost, "/api/nodes/1/enable", `{}`, "test-token-123456")
	if resp.Code != http.StatusOK {
		t.Fatalf("enable failed: %d %s", resp.Code, resp.Body.String())
	}
	st, err := state.Load(app.Paths.StatePath())
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if len(st.Nodes) != 1 || !st.Nodes[0].Enabled {
		t.Fatalf("expected imported node to be enabled: %+v", st.Nodes)
	}
}

func TestWebUIConfigUpdateSavesSafeFields(t *testing.T) {
	app, _ := newTestApp(t)
	handler := newWebUIHandler(app, "test-token-123456")
	body := `{
		"controller_bind_address":"0.0.0.0",
		"lan_cidrs":["192.168.2.0/24",""],
		"lan_allowed_cidrs":["100.64.0.0/10"],
		"core_amd64_cpu_level":"v3",
		"cors_allow_private_network":true
	}`
	resp := authedRequest(t, handler, http.MethodPost, "/api/config", body, "test-token-123456")
	if resp.Code != http.StatusOK {
		t.Fatalf("config update failed: %d %s", resp.Code, resp.Body.String())
	}
	cfg, err := config.Load(app.Paths.ConfigPath())
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Controller.BindAddress != "0.0.0.0" || cfg.Install.CoreAMD64CPULevel != "v3" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
	if len(cfg.Network.LANCIDRs) != 1 || len(cfg.Access.LANAllowedCIDRs) != 1 {
		t.Fatalf("expected cleaned cidr lists: %+v", cfg)
	}
	if !cfg.Controller.CORSAllowPrivateNetwork {
		t.Fatalf("expected CORS private network flag to be saved")
	}
}

func TestWebUIDefaultsToLANAndRequiresStrongToken(t *testing.T) {
	if defaultWebUIAddr != "0.0.0.0:18080" {
		t.Fatalf("unexpected default webui addr: %q", defaultWebUIAddr)
	}
	if webUIListenNetwork(defaultWebUIAddr) != "tcp4" {
		t.Fatalf("default webui addr must use tcp4, got %q", webUIListenNetwork(defaultWebUIAddr))
	}
	if webUIAddrIsLoopback(defaultWebUIAddr) {
		t.Fatalf("default webui addr must be LAN reachable")
	}
	if webUIAddrIsLoopback("0.0.0.0:18080") {
		t.Fatalf("0.0.0.0 must not be considered loopback")
	}
	if !webUIAddrIsLoopback("127.0.0.1:18080") {
		t.Fatalf("127.0.0.1 must be loopback")
	}
	if webUITokenStrong("minimalist-secret") {
		t.Fatalf("fallback secret must not be accepted as strong")
	}
	if !webUITokenStrong("0123456789abcdef") {
		t.Fatalf("expected long token to be strong")
	}
	if err := validateWebUIExposure(defaultWebUIAddr, "minimalist-secret"); err == nil || !strings.Contains(err.Error(), "weak token") {
		t.Fatalf("expected weak token LAN guard, got %v", err)
	}
	if err := validateWebUIExposure(defaultWebUIAddr, "0123456789abcdef"); err != nil {
		t.Fatalf("expected strong token LAN exposure to pass: %v", err)
	}
	if err := validateWebUIExposure("127.0.0.1:18080", "weak"); err != nil {
		t.Fatalf("loopback exposure should allow short development tokens: %v", err)
	}
}

func authedRequest(t *testing.T, handler http.Handler, method, path, body, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("X-Minimalist-Token", token)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	return resp
}

func decodeWebResponse(t *testing.T, resp *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode json: %v\n%s", err, resp.Body.String())
	}
	if ok, _ := body["ok"].(bool); !ok {
		t.Fatalf("expected ok response: %#v", body)
	}
	return body
}
