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
			"v":   "2",
			"ps":  name,
			"add": "example.com",
			"port": "443",
			"id":  "12345678-1234-1234-1234-1234567890ab",
			"net": "ws",
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
