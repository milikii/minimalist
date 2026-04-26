package provider

import (
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
