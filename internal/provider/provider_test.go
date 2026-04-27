package provider

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	yaml "gopkg.in/yaml.v2"

	"minimalist/internal/state"
)

func TestScanAndRenderProvider(t *testing.T) {
	rows := ScanURIRows(strings.Join([]string{
		"vless://12345678-1234-1234-1234-1234567890ab@example.com:443?encryption=none&security=tls&type=ws&host=cdn.example.com&path=%2Fws#vless-node",
		"trojan://password@example.org:443?security=tls#trojan-node",
		"ss://YWVzLTI1Ni1nY206c2VjcmV0QGV4YW1wbGUubmV0OjQ0Mw==#ss-node",
	}, "\n"))
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
	nodes := []state.Node{}
	nodes = AppendImportedNodes(nodes, rows, "manual", "", true)
	if len(nodes) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(nodes))
	}
	out := filepath.Join(t.TempDir(), "manual.txt")
	if err := RenderProvider(out, nodes, "", "subscription"); err != nil {
		t.Fatalf("render provider: %v", err)
	}
	body, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read provider: %v", err)
	}
	text := string(body)
	for _, needle := range []string{"type: vless", "type: trojan", "type: ss"} {
		if !strings.Contains(text, needle) {
			t.Fatalf("missing %q in provider output:\n%s", needle, text)
		}
	}
}

func TestRenderProviderFiltersAndHandlesEmptyAndUnsupportedNodes(t *testing.T) {
	dir := t.TempDir()
	manualPath := filepath.Join(dir, "manual.txt")
	nodes := []state.Node{
		{
			ID:         "1",
			Name:       "manual-node",
			Enabled:    true,
			URI:        "trojan://password@example.org:443?security=tls#manual-node",
			ImportedAt: state.NowISO(),
			Source:     state.Source{Kind: "manual"},
		},
		{
			ID:         "2",
			Name:       "disabled-node",
			Enabled:    false,
			URI:        "trojan://password@example.org:443?security=tls#disabled-node",
			ImportedAt: state.NowISO(),
			Source:     state.Source{Kind: "manual"},
		},
		{
			ID:         "3",
			Name:       "subscription-node",
			Enabled:    true,
			URI:        "trojan://password@example.org:443?security=tls#subscription-node",
			ImportedAt: state.NowISO(),
			Source:     state.Source{Kind: "subscription", ID: "sub-1"},
		},
	}
	if err := RenderProvider(manualPath, nodes, "manual", "subscription"); err != nil {
		t.Fatalf("render filtered provider: %v", err)
	}
	body, err := os.ReadFile(manualPath)
	if err != nil {
		t.Fatalf("read filtered provider: %v", err)
	}
	text := string(body)
	if !strings.Contains(text, "manual-node") {
		t.Fatalf("expected manual node in provider output:\n%s", text)
	}
	if strings.Contains(text, "disabled-node") || strings.Contains(text, "subscription-node") {
		t.Fatalf("expected filters to exclude disabled/subscription nodes:\n%s", text)
	}

	emptyPath := filepath.Join(dir, "empty.txt")
	if err := RenderProvider(emptyPath, nil, "", "subscription"); err != nil {
		t.Fatalf("render empty provider: %v", err)
	}
	emptyBody, err := os.ReadFile(emptyPath)
	if err != nil {
		t.Fatalf("read empty provider: %v", err)
	}
	if !strings.Contains(string(emptyBody), "proxies: []") {
		t.Fatalf("expected empty provider output, got:\n%s", string(emptyBody))
	}

	if err := RenderProvider(filepath.Join(dir, "bad.txt"), []state.Node{{
		ID:         "4",
		Name:       "bad-node",
		Enabled:    true,
		URI:        "socks5://proxy.example.com:1080#bad-node",
		ImportedAt: state.NowISO(),
		Source:     state.Source{Kind: "manual"},
	}}, "", ""); err == nil || !strings.Contains(err.Error(), "unsupported scheme") {
		t.Fatalf("expected unsupported scheme error, got %v", err)
	}
}

func TestDecodeSubscriptionLinesAndScannableURIsHandleBase64Payload(t *testing.T) {
	payload := base64.StdEncoding.EncodeToString([]byte(strings.Join([]string{
		"trojan://password@example.org:443?security=tls#one",
		"not-a-uri",
		"ss://YWVzLTI1Ni1nY206c2VjcmV0QGV4YW1wbGUubmV0OjQ0Mw==#two",
	}, "\n")))
	lines := DecodeSubscriptionLines(payload)
	if len(lines) != 3 {
		t.Fatalf("expected decoded lines, got %#v", lines)
	}
	uris := ScannableSubscriptionURIs(payload)
	if len(uris) != 2 {
		t.Fatalf("expected 2 scannable uris, got %#v", uris)
	}
}

func TestDecodeSubscriptionLinesHandlesWrappedUnpaddedBase64Payload(t *testing.T) {
	raw := "trojan://password@example.org:443?security=tls#wrapped\n"
	payload := strings.TrimRight(base64.StdEncoding.EncodeToString([]byte(raw)), "=")
	wrapped := payload[:12] + "\n  " + payload[12:] + "\n"
	lines := DecodeSubscriptionLines(wrapped)
	if len(lines) != 1 || lines[0] != strings.TrimSpace(raw) {
		t.Fatalf("unexpected decoded lines: %#v", lines)
	}
	uris := ScannableSubscriptionURIs(wrapped)
	if len(uris) != 1 || uris[0] != strings.TrimSpace(raw) {
		t.Fatalf("unexpected scannable uris: %#v", uris)
	}
}

func TestDecodeSubscriptionLinesFallsBackToRawPlainText(t *testing.T) {
	payload := "not-base64\n  trojan://password@example.org:443?security=tls#raw-node  \nplain-text"
	lines := DecodeSubscriptionLines(payload)
	if len(lines) != 3 || lines[0] != "not-base64" || lines[1] != "trojan://password@example.org:443?security=tls#raw-node" || lines[2] != "plain-text" {
		t.Fatalf("unexpected raw fallback lines: %#v", lines)
	}
	uris := ScannableSubscriptionURIs(payload)
	if len(uris) != 1 || uris[0] != "trojan://password@example.org:443?security=tls#raw-node" {
		t.Fatalf("unexpected scannable uris: %#v", uris)
	}
}

func TestAppendImportedNodesDeduplicatesByBaseKeyAndRenamesConflicts(t *testing.T) {
	existing := []state.Node{{
		ID:         "1",
		Name:       "dup-node",
		Enabled:    true,
		URI:        "trojan://password@example.org:443?security=tls#old-name",
		ImportedAt: state.NowISO(),
		Source:     state.Source{Kind: "manual"},
	}}
	rows := []ScanRow{
		{
			URI:       "trojan://password@example.org:443?security=tls#new-name",
			Name:      "dup-node",
			Supported: "1",
		},
		{
			URI:       "trojan://password@another.example.org:443?security=tls#new-name",
			Name:      "dup-node",
			Supported: "1",
		},
	}
	nodes := AppendImportedNodes(existing, rows, "subscription", "sub-1", true)
	if len(nodes) != 2 {
		t.Fatalf("expected one new node after dedupe, got %#v", nodes)
	}
	if nodes[1].Name != "dup-node-2" {
		t.Fatalf("expected renamed node, got %q", nodes[1].Name)
	}
	if nodes[1].Source.Kind != "subscription" || nodes[1].Source.ID != "sub-1" || !nodes[1].Enabled {
		t.Fatalf("unexpected appended node: %#v", nodes[1])
	}
}

func TestURIBaseKeyIgnoresVMessPSField(t *testing.T) {
	makeVMess := func(name string) string {
		payload, err := json.Marshal(map[string]any{
			"v":    "2",
			"ps":   name,
			"add":  "example.com",
			"port": "443",
			"id":   "12345678-1234-1234-1234-1234567890ab",
			"net":  "ws",
			"path": "/ws",
		})
		if err != nil {
			t.Fatalf("marshal vmess: %v", err)
		}
		return "vmess://" + base64.StdEncoding.EncodeToString(payload)
	}
	if URIBaseKey(makeVMess("one")) != URIBaseKey(makeVMess("two")) {
		t.Fatalf("expected vmess base key to ignore ps field")
	}
}

func TestURIBaseKeyHandlesInvalidURIsAndDropsFragment(t *testing.T) {
	raw := "trojan://secret@example.com:443?security=tls#display-name"
	if got := URIBaseKey(raw); got != "trojan://secret@example.com:443?security=tls" {
		t.Fatalf("expected fragment-free base key, got %q", got)
	}
	if got := URIBaseKey("vmess://@@@"); got != "vmess://@@@" {
		t.Fatalf("expected invalid vmess base key to stay raw, got %q", got)
	}
	if got := URIBaseKey("%"); got != "%" {
		t.Fatalf("expected invalid URI base key to stay raw, got %q", got)
	}
}

func TestGuessNamePrefersFragmentAndVMessPS(t *testing.T) {
	vmessPayload, err := json.Marshal(map[string]any{
		"v":    "2",
		"ps":   "named-vmess",
		"add":  "example.com",
		"port": "443",
		"id":   "12345678-1234-1234-1234-1234567890ab",
	})
	if err != nil {
		t.Fatalf("marshal vmess: %v", err)
	}
	if name := GuessName("vmess://" + base64.StdEncoding.EncodeToString(vmessPayload)); name != "named-vmess" {
		t.Fatalf("expected vmess ps name, got %q", name)
	}
	if name := GuessName("trojan://password@example.org:443?type=grpc#trojan-fragment"); name != "trojan-fragment" {
		t.Fatalf("expected fragment name, got %q", name)
	}
	if name := GuessName("vmess://@@@"); name != "vmess-node" {
		t.Fatalf("expected vmess fallback name, got %q", name)
	}
}

func TestGuessNameCoversFallbacks(t *testing.T) {
	vmessPayload, err := json.Marshal(map[string]any{
		"add": "edge.example.com",
		"net": "ws",
	})
	if err != nil {
		t.Fatalf("marshal vmess: %v", err)
	}
	if name := GuessName("vmess://" + base64.StdEncoding.EncodeToString(vmessPayload)); name != "ws-edge.example.com" {
		t.Fatalf("expected vmess host fallback name, got %q", name)
	}
	if name := GuessName("%"); name != "node" {
		t.Fatalf("expected invalid uri fallback name, got %q", name)
	}
	if name := GuessName("ss://secret@example.com:443"); name != "ss-example.com" {
		t.Fatalf("expected ss host fallback name, got %q", name)
	}
	if name := GuessName("trojan://secret@:443"); name != "tcp-node" {
		t.Fatalf("expected missing host fallback name, got %q", name)
	}
}

func TestURIHelpersExposeSchemeHostPortAndQuery(t *testing.T) {
	raw := "  VLESS://12345678-1234-1234-1234-1234567890ab@edge.example.com:8443?type=ws&security=tls  "
	if scheme := uriScheme(raw); scheme != "vless" {
		t.Fatalf("expected vless scheme, got %q", scheme)
	}
	if host := splitHost(raw); host != "edge.example.com" {
		t.Fatalf("expected edge.example.com host, got %q", host)
	}
	if port := splitPort(raw); port != "8443" {
		t.Fatalf("expected 8443 port, got %q", port)
	}
	if network := queryField(raw, "type", "tcp"); network != "ws" {
		t.Fatalf("expected ws query field, got %q", network)
	}
	if fallback := queryField("::not a uri::", "type", "tcp"); fallback != "tcp" {
		t.Fatalf("expected fallback query field, got %q", fallback)
	}
	if scheme := uriScheme("not-a-uri"); scheme != "" {
		t.Fatalf("expected empty scheme, got %q", scheme)
	}
	if host := splitHost("%"); host != "" {
		t.Fatalf("expected empty host for invalid uri, got %q", host)
	}
	if port := splitPort("%"); port != "" {
		t.Fatalf("expected empty port for invalid uri, got %q", port)
	}
}

func TestParseSSSupportsBase64PrefixAndPluginOptions(t *testing.T) {
	info, err := parseSS("ss://YWVzLTI1Ni1nY206c2VjcmV0@example.com:443?plugin=obfs-local%3Bobfs%3Dhttp%3Bobfs-host%3Dcdn.example%3Btls%3D1#mynode")
	if err != nil {
		t.Fatalf("parse ss: %v", err)
	}
	if info.Cipher != "aes-256-gcm" || info.Password != "secret" {
		t.Fatalf("unexpected ss credentials: %#v", info)
	}
	if info.Server != "example.com" || info.Port != 443 {
		t.Fatalf("unexpected ss endpoint: %#v", info)
	}
	if info.Plugin != "obfs-local" {
		t.Fatalf("expected obfs-local plugin, got %#v", info.Plugin)
	}
	if got := info.PluginOpts["obfs"]; got != "http" {
		t.Fatalf("expected obfs option, got %#v", got)
	}
	if got := info.PluginOpts["obfs-host"]; got != "cdn.example" {
		t.Fatalf("expected obfs-host option, got %#v", got)
	}
	if got := info.PluginOpts["tls"]; got != true {
		t.Fatalf("expected tls bool option, got %#v", got)
	}
}

func TestParseSSSupportsPlainAuthority(t *testing.T) {
	info, err := parseSS("ss://aes-128-gcm:secret@example.com:8388#plain")
	if err != nil {
		t.Fatalf("parse ss plain authority: %v", err)
	}
	if info.Cipher != "aes-128-gcm" || info.Password != "secret" {
		t.Fatalf("unexpected ss credentials: %#v", info)
	}
	if info.Server != "example.com" || info.Port != 8388 {
		t.Fatalf("unexpected ss endpoint: %#v", info)
	}
}

func TestDecodeSSAuthorityRejectsInvalidPayload(t *testing.T) {
	if _, _, _, _, err := decodeSSAuthority("ss://not-valid"); err == nil {
		t.Fatalf("expected invalid ss uri error")
	}
}

func TestProtocolParsersRejectInvalidURIs(t *testing.T) {
	tests := []struct {
		name string
		run  func() error
	}{
		{"vless", func() error {
			_, err := parseVless("vless://12345678-1234-1234-1234-1234567890ab@example.com")
			return err
		}},
		{"trojan", func() error {
			_, err := parseTrojan("trojan://secret@example.com")
			return err
		}},
		{"ss", func() error {
			_, err := parseSS("ss://YWVzLTI1Ni1nY206c2VjcmV0")
			return err
		}},
		{"vmess", func() error {
			_, err := parseVMess("vmess://not-valid")
			return err
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.run(); err == nil {
				t.Fatalf("expected invalid %s uri error", tc.name)
			}
		})
	}
}

func TestParseVMessAndBuildProviderKeepTLSAndGRPCFields(t *testing.T) {
	payload, err := json.Marshal(map[string]any{
		"v":           "2",
		"ps":          "vmess-grpc",
		"add":         "vmess.example.com",
		"port":        "443",
		"id":          "12345678-1234-1234-1234-1234567890ab",
		"net":         "grpc",
		"path":        "/ignored",
		"tls":         "tls",
		"sni":         "edge.example.com",
		"alpn":        "h2,http/1.1",
		"fp":          "chrome",
		"insecure":    "1",
		"serviceName": "grpc-svc",
	})
	if err != nil {
		t.Fatalf("marshal vmess: %v", err)
	}
	info, err := parseVMess("vmess://" + base64.StdEncoding.EncodeToString(payload))
	if err != nil {
		t.Fatalf("parse vmess: %v", err)
	}
	if info.Server != "vmess.example.com" || info.UUID != "12345678-1234-1234-1234-1234567890ab" {
		t.Fatalf("unexpected vmess identity: %#v", info)
	}
	if !info.TLS || info.ServiceName != "grpc-svc" {
		t.Fatalf("expected tls grpc info, got %#v", info)
	}
	item := buildVMessProvider("vmess-grpc", info)
	if !item.TLS || item.Network != "grpc" {
		t.Fatalf("unexpected vmess provider flags: %#v", item)
	}
	if item.GRPCOpts["grpc-service-name"] != "grpc-svc" {
		t.Fatalf("expected grpc service name, got %#v", item.GRPCOpts)
	}
	if item.ServerName != "edge.example.com" || item.Fingerprint != "chrome" {
		t.Fatalf("unexpected tls fields: %#v", item)
	}
	if item.SkipCertVerify == nil || !*item.SkipCertVerify {
		t.Fatalf("expected skip cert verify to be true: %#v", item)
	}
}

func TestProviderItemFromNodeDispatchesSupportedSchemes(t *testing.T) {
	vmessPayload, err := json.Marshal(map[string]any{
		"add":  "vmess.example.com",
		"port": "443",
		"id":   "12345678-1234-1234-1234-1234567890ab",
	})
	if err != nil {
		t.Fatalf("marshal vmess: %v", err)
	}
	tests := []struct {
		name string
		uri  string
		want string
	}{
		{"vless", "vless://12345678-1234-1234-1234-1234567890ab@example.com:443?encryption=none#vless", "vless"},
		{"ss", "ss://YWVzLTI1Ni1nY206c2VjcmV0QGV4YW1wbGUubmV0OjQ0Mw==#ss", "ss"},
		{"vmess", "vmess://" + base64.StdEncoding.EncodeToString(vmessPayload), "vmess"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			item, err := providerItemFromNode(state.Node{Name: tc.name, URI: tc.uri})
			if err != nil {
				t.Fatalf("provider item from node: %v", err)
			}
			raw, err := yaml.Marshal(item)
			if err != nil {
				t.Fatalf("marshal provider item: %v", err)
			}
			if !strings.Contains(string(raw), "type: "+tc.want+"\n") {
				t.Fatalf("expected provider type %s, got:\n%s", tc.want, string(raw))
			}
		})
	}
}

func TestParseVMessRejectsMissingIdentity(t *testing.T) {
	payload, err := json.Marshal(map[string]any{
		"add":  "vmess.example.com",
		"port": "443",
	})
	if err != nil {
		t.Fatalf("marshal vmess: %v", err)
	}
	if _, err := parseVMess("vmess://" + base64.StdEncoding.EncodeToString(payload)); err == nil {
		t.Fatalf("expected invalid vmess uri")
	}
}

func TestParseVlessCapturesRealityAndXHTTPDownloadSettings(t *testing.T) {
	extra := `{"downloadSettings":{"address":"download.example.com","port":"8443","security":"reality","realitySettings":{"publicKey":"pub","shortId":"sid","spiderX":"/spider","serverName":"download-sni","fingerprint":"chrome"},"xhttpSettings":{"path":"/dl","host":"dl.example.com","mode":"stream-up"}}}`
	info, err := parseVless("vless://12345678-1234-1234-1234-1234567890ab@example.com:443?type=xhttp&mode=packet-up&host=cdn.example.com&path=%2Fproxy&pbk=main-pub&sid=main-sid&spx=%2Fmain&extra=" + extra)
	if err != nil {
		t.Fatalf("parse vless: %v", err)
	}
	if info.Network != "xhttp" || info.Mode != "packet-up" {
		t.Fatalf("unexpected xhttp fields: %#v", info)
	}
	if info.RealityOpts["public-key"] != "main-pub" || info.RealityOpts["short-id"] != "main-sid" {
		t.Fatalf("unexpected reality opts: %#v", info.RealityOpts)
	}
	if info.DownloadSetting["server"] != "download.example.com" || info.DownloadSetting["port"] != 8443 {
		t.Fatalf("unexpected download settings: %#v", info.DownloadSetting)
	}
	if info.DownloadSetting["tls"] != true || info.DownloadSetting["servername"] != "download-sni" {
		t.Fatalf("unexpected download tls settings: %#v", info.DownloadSetting)
	}
	reality, ok := info.DownloadSetting["reality-opts"].(map[string]any)
	if !ok || reality["public-key"] != "pub" || reality["spider-x"] != "/spider" {
		t.Fatalf("unexpected nested reality opts: %#v", info.DownloadSetting)
	}
}

func TestParseVlessPreservesSkipCertVerifyAlias(t *testing.T) {
	info, err := parseVless("vless://12345678-1234-1234-1234-1234567890ab@example.com:443?type=ws&host=cdn.example.com&path=%2Fws&skip-cert-verify=1")
	if err != nil {
		t.Fatalf("parse vless: %v", err)
	}
	if info.SkipCertVerify == nil || !*info.SkipCertVerify {
		t.Fatalf("expected vless skip-cert-verify to be true: %#v", info)
	}
	item := buildVlessProvider("vless-ws", info)
	if item.SkipCertVerify == nil || !*item.SkipCertVerify {
		t.Fatalf("expected provider item to preserve skip-cert-verify: %#v", item)
	}
}

func TestProviderHelpersCoverTypedValuesAndXHTTPDownloadAliases(t *testing.T) {
	if anyString(float64(8443)) != "8443" || anyString(443) != "443" || anyString(nil) != "" {
		t.Fatalf("unexpected anyString conversions")
	}
	if intFromAny(8080) != 8080 || intFromAny(float64(8443)) != 8443 || intFromAny("443") != 443 || intFromAny("bad") != 0 {
		t.Fatalf("unexpected intFromAny conversions")
	}
	if firstNonEmpty("", "", "fallback") != "fallback" {
		t.Fatalf("expected first non-empty value")
	}
	if got := xhttpDownloadSettings(map[string]any{}); got != nil {
		t.Fatalf("expected empty xhttp download settings to stay nil, got %#v", got)
	}

	settings := xhttpDownloadSettings(map[string]any{
		"server":   "download.example.com",
		"port":     float64(8443),
		"security": "reality",
		"xhttpSettings": map[string]any{
			"path": "/dl",
			"host": "dl.example.com",
			"mode": "packet-up",
		},
		"realitySettings": map[string]any{
			"public-key": "pub",
			"short-id":   "sid",
			"spider-x":   "/spider",
			"sni":        "reality.example.com",
			"fp":         "firefox",
		},
	})
	for key, want := range map[string]any{
		"path":               "/dl",
		"host":               "dl.example.com",
		"mode":               "packet-up",
		"server":             "download.example.com",
		"port":               8443,
		"tls":                true,
		"servername":         "reality.example.com",
		"client-fingerprint": "firefox",
	} {
		if settings[key] != want {
			t.Fatalf("expected %s=%#v, got %#v in %#v", key, want, settings[key], settings)
		}
	}
	reality, ok := settings["reality-opts"].(map[string]any)
	if !ok || reality["public-key"] != "pub" || reality["short-id"] != "sid" || reality["spider-x"] != "/spider" {
		t.Fatalf("unexpected reality opts: %#v", settings)
	}

	plugin, opts := parseSSPlugin("obfs-local; mux = true ; no-value ; host = cdn.example")
	if plugin != "obfs-local" || opts["mux"] != true || opts["host"] != "cdn.example" {
		t.Fatalf("unexpected ss plugin opts: plugin=%q opts=%#v", plugin, opts)
	}
	if _, ok := opts["no-value"]; ok {
		t.Fatalf("did not expect no-value option to be recorded: %#v", opts)
	}
}

func TestBuildVlessProviderIncludesXHTTPOptions(t *testing.T) {
	info := uriInfo{
		Scheme:  "vless",
		Server:  "example.com",
		Port:    443,
		UUID:    "12345678-1234-1234-1234-1234567890ab",
		Network: "xhttp",
		Path:    "/proxy",
		Host:    "cdn.example.com",
		Mode:    "packet-up",
		DownloadSetting: map[string]any{
			"server": "download.example.com",
			"port":   8443,
		},
		RealityOpts: map[string]any{"public-key": "pub"},
	}
	item := buildVlessProvider("xhttp-node", info)
	if !item.TLS {
		t.Fatalf("expected reality-backed vless provider to enable tls: %#v", item)
	}
	if item.XHTTPOpts["path"] != "/proxy" || item.XHTTPOpts["host"] != "cdn.example.com" || item.XHTTPOpts["mode"] != "packet-up" {
		t.Fatalf("unexpected xhttp opts: %#v", item.XHTTPOpts)
	}
	download, ok := item.XHTTPOpts["download-settings"].(map[string]any)
	if !ok || download["server"] != "download.example.com" {
		t.Fatalf("unexpected xhttp download settings: %#v", item.XHTTPOpts)
	}
}

func TestApplyNetworkFieldsCoversWSHTTPUpgradeH2AndTCPHeader(t *testing.T) {
	ws := buildVlessProvider("ws-node", uriInfo{
		Scheme:  "vless",
		Server:  "example.com",
		Port:    443,
		UUID:    "12345678-1234-1234-1234-1234567890ab",
		Network: "ws",
		Path:    "/ws",
		Host:    "cdn.example.com",
	})
	if ws.WSOpts["path"] != "/ws" {
		t.Fatalf("expected ws path, got %#v", ws.WSOpts)
	}
	headers, ok := ws.WSOpts["headers"].(map[string]any)
	if !ok || headers["Host"] != "cdn.example.com" {
		t.Fatalf("expected ws host header, got %#v", ws.WSOpts)
	}

	upgrade := buildTrojanProvider("upgrade-node", uriInfo{
		Scheme:     "trojan",
		Server:     "example.com",
		Port:       443,
		Password:   "secret",
		Network:    "httpupgrade",
		Path:       "/up",
		Host:       "upgrade.example.com",
		ServerName: "upgrade.example.com",
	})
	if upgrade.HTTPUpgradeOpts["path"] != "/up" || upgrade.HTTPUpgradeOpts["host"] != "upgrade.example.com" {
		t.Fatalf("unexpected httpupgrade opts: %#v", upgrade.HTTPUpgradeOpts)
	}

	h2 := buildVMessProvider("h2-node", uriInfo{
		Scheme:  "vmess",
		Server:  "example.com",
		Port:    443,
		UUID:    "12345678-1234-1234-1234-1234567890ab",
		Network: "h2",
		Path:    "/h2",
		Host:    "h2.example.com",
	})
	if hosts, ok := h2.H2Opts["host"].([]string); !ok || len(hosts) != 1 || hosts[0] != "h2.example.com" {
		t.Fatalf("unexpected h2 host opts: %#v", h2.H2Opts)
	}
	if h2.H2Opts["path"] != "/h2" {
		t.Fatalf("unexpected h2 path opts: %#v", h2.H2Opts)
	}

	tcp := buildTrojanProvider("tcp-node", uriInfo{
		Scheme:     "trojan",
		Server:     "example.com",
		Port:       443,
		Password:   "secret",
		Network:    "tcp",
		HeaderType: "http",
	})
	if tcp.Header["type"] != "http" {
		t.Fatalf("unexpected tcp header opts: %#v", tcp.Header)
	}
}

func TestParseTrojanAndProviderItemPreserveTLSFields(t *testing.T) {
	info, err := parseTrojan("trojan://secret@example.com:443?type=grpc&serviceName=svc&sni=edge.example.com&alpn=h2,http/1.1&fp=chrome&allowInsecure=yes")
	if err != nil {
		t.Fatalf("parse trojan: %v", err)
	}
	if info.Network != "grpc" || info.ServiceName != "svc" {
		t.Fatalf("unexpected trojan network fields: %#v", info)
	}
	if info.ServerName != "edge.example.com" || info.Fingerprint != "chrome" {
		t.Fatalf("unexpected trojan tls fields: %#v", info)
	}
	if info.SkipCertVerify == nil || !*info.SkipCertVerify {
		t.Fatalf("expected trojan skip-cert-verify to be true: %#v", info)
	}

	itemAny, err := providerItemFromNode(state.Node{
		Name: "trojan-grpc",
		URI:  "trojan://secret@example.com:443?type=grpc&serviceName=svc&sni=edge.example.com&alpn=h2,http/1.1&fp=chrome&allowInsecure=yes",
	})
	if err != nil {
		t.Fatalf("provider item from node: %v", err)
	}
	item, ok := itemAny.(trojanProvider)
	if !ok {
		t.Fatalf("expected trojan provider item, got %#v", itemAny)
	}
	if !item.TLS || item.GRPCOpts["grpc-service-name"] != "svc" {
		t.Fatalf("unexpected trojan provider item: %#v", item)
	}
}

func TestRenderProviderFiltersManualAndSubscriptionSources(t *testing.T) {
	out := filepath.Join(t.TempDir(), "provider.yaml")
	nodes := []state.Node{
		{
			Name:    "manual-node",
			Enabled: true,
			URI:     "trojan://secret@example.com:443?security=tls#manual-node",
			Source:  state.Source{Kind: "manual"},
		},
		{
			Name:    "subscription-node",
			Enabled: true,
			URI:     "trojan://secret@subscription.example.com:443?security=tls#subscription-node",
			Source:  state.Source{Kind: "subscription", ID: "sub-1"},
		},
	}
	if err := RenderProvider(out, nodes, "", "subscription"); err != nil {
		t.Fatalf("render provider: %v", err)
	}
	body, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read provider: %v", err)
	}
	var file providerFile
	if err := yaml.Unmarshal(body, &file); err != nil {
		t.Fatalf("unmarshal provider yaml: %v", err)
	}
	if len(file.Proxies) != 1 {
		t.Fatalf("expected only manual node after exclude filter, got %#v", file.Proxies)
	}
	rendered, ok := file.Proxies[0].(map[interface{}]interface{})
	if !ok || rendered["name"] != "manual-node" {
		t.Fatalf("unexpected rendered proxy: %#v", file.Proxies[0])
	}
}

func TestRenderProviderFiltersRequestedSourceKind(t *testing.T) {
	out := filepath.Join(t.TempDir(), "provider.yaml")
	nodes := []state.Node{
		{
			Name:    "manual-node",
			Enabled: true,
			URI:     "trojan://secret@example.com:443?security=tls#manual-node",
			Source:  state.Source{Kind: "manual"},
		},
		{
			Name:    "subscription-node",
			Enabled: true,
			URI:     "trojan://secret@subscription.example.com:443?security=tls#subscription-node",
			Source:  state.Source{Kind: "subscription", ID: "sub-1"},
		},
	}
	if err := RenderProvider(out, nodes, "subscription", ""); err != nil {
		t.Fatalf("render provider: %v", err)
	}
	body, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read provider: %v", err)
	}
	var file providerFile
	if err := yaml.Unmarshal(body, &file); err != nil {
		t.Fatalf("unmarshal provider yaml: %v", err)
	}
	if len(file.Proxies) != 1 {
		t.Fatalf("expected only subscription node after source filter, got %#v", file.Proxies)
	}
	rendered, ok := file.Proxies[0].(map[interface{}]interface{})
	if !ok || rendered["name"] != "subscription-node" {
		t.Fatalf("unexpected rendered proxy: %#v", file.Proxies[0])
	}
}

func TestRenderProviderReturnsErrorWhenParentPathIsBlocked(t *testing.T) {
	root := t.TempDir()
	blockedDir := filepath.Join(root, "blocked")
	if err := os.WriteFile(blockedDir, []byte("blocked"), 0o640); err != nil {
		t.Fatalf("write blocking file: %v", err)
	}
	err := RenderProvider(filepath.Join(blockedDir, "provider.yaml"), []state.Node{{
		Name:    "manual-node",
		Enabled: true,
		URI:     "trojan://secret@example.com:443?security=tls#manual-node",
		Source:  state.Source{Kind: "manual"},
	}}, "", "")
	if err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("expected blocked path error, got %v", err)
	}
}

func TestRenderProviderSkipsDisabledNodes(t *testing.T) {
	out := filepath.Join(t.TempDir(), "provider.yaml")
	nodes := []state.Node{
		{
			Name:    "disabled-node",
			Enabled: false,
			URI:     "trojan://secret@example.com:443?security=tls#disabled-node",
			Source:  state.Source{Kind: "manual"},
		},
	}
	if err := RenderProvider(out, nodes, "", ""); err != nil {
		t.Fatalf("render provider: %v", err)
	}
	body, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read provider: %v", err)
	}
	var file providerFile
	if err := yaml.Unmarshal(body, &file); err != nil {
		t.Fatalf("unmarshal provider yaml: %v", err)
	}
	if len(file.Proxies) != 0 {
		t.Fatalf("expected disabled nodes to be skipped, got %#v", file.Proxies)
	}
}

func TestProviderHelpersNormalizePrimitiveValues(t *testing.T) {
	if !truthy("YES") || truthy("0") {
		t.Fatalf("unexpected truthy behavior")
	}
	if anyString(12.0) != "12" || anyString(7) != "7" || anyString(true) != "" {
		t.Fatalf("unexpected anyString conversions")
	}
	if intFromAny("9") != 9 || intFromAny(3.0) != 3 || intFromAny(false) != 0 {
		t.Fatalf("unexpected intFromAny conversions")
	}
	if got := splitLines(" a \n\nb\n "); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("unexpected split lines: %#v", got)
	}
	if firstNonEmpty("", "fallback", "ignored") != "fallback" {
		t.Fatalf("unexpected firstNonEmpty result")
	}
	if got := splitCSV("h2, http/1.1, ,grpc"); len(got) != 3 || got[1] != "http/1.1" {
		t.Fatalf("unexpected splitCSV result: %#v", got)
	}
}

func TestScanURIRowMarksUnsupportedScheme(t *testing.T) {
	row := ScanURIRow("socks5://proxy.example.com:1080#legacy")
	if row.Supported != "0" {
		t.Fatalf("expected unsupported row: %#v", row)
	}
	if row.Scheme != "socks5" || row.Name != "legacy" {
		t.Fatalf("unexpected unsupported row metadata: %#v", row)
	}
	if !strings.Contains(row.Reason, "unsupported scheme") {
		t.Fatalf("expected unsupported reason, got %#v", row)
	}
}

func TestSecurityNameReflectsProtocolSpecificRules(t *testing.T) {
	if got := securityName(uriInfo{Scheme: "vless"}); got != "vless" {
		t.Fatalf("expected plain vless security, got %q", got)
	}
	if got := securityName(uriInfo{Scheme: "vless", ServerName: "edge.example.com"}); got != "tls" {
		t.Fatalf("expected vless tls security, got %q", got)
	}
	if got := securityName(uriInfo{Scheme: "vless", RealityOpts: map[string]any{"public-key": "pub"}}); got != "reality" {
		t.Fatalf("expected vless reality security, got %q", got)
	}
	if got := securityName(uriInfo{Scheme: "trojan"}); got != "tls" {
		t.Fatalf("expected trojan tls security, got %q", got)
	}
	if got := securityName(uriInfo{Scheme: "trojan", Plugin: "obfs-local"}); got != "obfs-local" {
		t.Fatalf("expected trojan plugin security, got %q", got)
	}
	if got := securityName(uriInfo{Scheme: "vmess", TLS: true}); got != "tls" {
		t.Fatalf("expected vmess tls security, got %q", got)
	}
	if got := securityName(uriInfo{Scheme: "ss"}); got != "ss" {
		t.Fatalf("expected ss security, got %q", got)
	}
	if got := securityName(uriInfo{Scheme: "unknown"}); got != "unknown" {
		t.Fatalf("expected unknown scheme passthrough, got %q", got)
	}
}

func TestParseVlessSupportsDashedDownloadSettingsKey(t *testing.T) {
	extra := `{"download-settings":{"server":"download.example.com","port":8443,"security":"tls","serverName":"download-sni","fingerprint":"chrome","xhttpSettings":{"path":"/dl","host":"dl.example.com"}}}`
	info, err := parseVless("vless://12345678-1234-1234-1234-1234567890ab@example.com:443?type=xhttp&extra=" + extra)
	if err != nil {
		t.Fatalf("parse vless: %v", err)
	}
	if info.DownloadSetting["server"] != "download.example.com" || info.DownloadSetting["port"] != 8443 {
		t.Fatalf("unexpected dashed download settings: %#v", info.DownloadSetting)
	}
	if info.DownloadSetting["servername"] != "download-sni" || info.DownloadSetting["client-fingerprint"] != "chrome" {
		t.Fatalf("unexpected dashed tls settings: %#v", info.DownloadSetting)
	}
}

func TestParseURIInfoAndProviderItemRejectUnsupportedSchemes(t *testing.T) {
	if _, err := parseURIInfo("socks5://proxy.example.com:1080"); err == nil {
		t.Fatalf("expected unsupported scheme error")
	}
	if _, err := providerItemFromNode(state.Node{Name: "bad", URI: "socks5://proxy.example.com:1080"}); err == nil {
		t.Fatalf("expected provider item error for unsupported scheme")
	}
}
