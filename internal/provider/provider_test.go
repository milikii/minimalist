package provider

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

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

func TestDecodeSSAuthorityRejectsInvalidPayload(t *testing.T) {
	if _, _, _, _, err := decodeSSAuthority("ss://not-valid"); err == nil {
		t.Fatalf("expected invalid ss uri error")
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
