package rulesrepo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitAndRenderDefaultRepo(t *testing.T) {
	root := filepath.Join(t.TempDir(), "rules-repo", "default")
	if err := InitDefaultRepo(root); err != nil {
		t.Fatalf("init repo: %v", err)
	}
	manifest := filepath.Join(root, "manifest.yaml")
	lines, err := Render(manifest)
	if err != nil {
		t.Fatalf("render repo: %v", err)
	}
	if len(lines) == 0 {
		t.Fatalf("expected rendered rules")
	}
	if !strings.Contains(strings.Join(lines, "\n"), "DOMAIN-SUFFIX,smzdm.com,DIRECT") {
		t.Fatalf("missing expected direct rule")
	}
	search, err := Search(manifest, "google")
	if err != nil {
		t.Fatalf("search repo: %v", err)
	}
	if len(search) < 2 {
		t.Fatalf("expected search results, got %#v", search)
	}
}

func TestLoadManifestAndReadEntriesValidateInputs(t *testing.T) {
	dir := t.TempDir()
	manifest := filepath.Join(dir, "manifest.yaml")
	if err := os.WriteFile(manifest, []byte("rulesets: []\n"), 0o640); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if _, err := LoadManifest(manifest); err == nil || !strings.Contains(err.Error(), "empty manifest") {
		t.Fatalf("expected empty manifest error, got %v", err)
	}
	if _, err := LoadManifest(filepath.Join(dir, "missing.yaml")); err == nil {
		t.Fatalf("expected missing manifest error")
	}
	if err := os.WriteFile(manifest, []byte("rulesets: [\n"), 0o640); err != nil {
		t.Fatalf("write invalid manifest: %v", err)
	}
	if _, err := LoadManifest(manifest); err == nil || !strings.Contains(err.Error(), "parse manifest") {
		t.Fatalf("expected parse manifest error, got %v", err)
	}
	entriesPath := filepath.Join(dir, "entries.txt")
	if err := os.WriteFile(entriesPath, []byte("# comment\nexample.com\n\nexample.com\n"), 0o640); err != nil {
		t.Fatalf("write entries: %v", err)
	}
	if _, err := ReadEntries(entriesPath); err == nil || !strings.Contains(err.Error(), "duplicate rule entry") {
		t.Fatalf("expected duplicate entry error, got %v", err)
	}
	if err := ValidateEntry("domain", "bad value", entriesPath); err == nil || !strings.Contains(err.Error(), "invalid domain entry") {
		t.Fatalf("expected invalid domain error, got %v", err)
	}
	if err := ValidateEntry("unsupported", "example.com", entriesPath); err == nil || !strings.Contains(err.Error(), "unsupported rule type") {
		t.Fatalf("expected unsupported rule type error, got %v", err)
	}
}

func TestCopyTreeAndRenderRejectUnsupportedManifestEntries(t *testing.T) {
	dir := t.TempDir()
	if err := copyTree("assets/missing", filepath.Join(dir, "dst")); err == nil {
		t.Fatalf("expected copyTree to fail for missing source")
	}

	manifest := filepath.Join(dir, "manifest.yaml")
	if err := os.WriteFile(manifest, []byte("rulesets:\n  - name: test\n    category: demo\n    type: invalid\n    source: entries.txt\n    target: direct\n"), 0o640); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "entries.txt"), []byte("example.com\n"), 0o640); err != nil {
		t.Fatalf("write entries: %v", err)
	}
	if _, err := Render(manifest); err == nil || !strings.Contains(err.Error(), "unsupported rule type in manifest") {
		t.Fatalf("expected unsupported type error, got %v", err)
	}

	if err := os.WriteFile(manifest, []byte("rulesets:\n  - name: test\n    category: demo\n    type: domain\n    source: entries.txt\n    target: invalid\n"), 0o640); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if _, err := Render(manifest); err == nil || !strings.Contains(err.Error(), "unsupported rule target in manifest") {
		t.Fatalf("expected unsupported target error, got %v", err)
	}
}

func TestFindRulesetAndDescribeRulesetReportMissingPaths(t *testing.T) {
	dir := t.TempDir()
	manifest := filepath.Join(dir, "manifest.yaml")
	if err := os.WriteFile(manifest, []byte("rulesets:\n  - name: test\n    category: demo\n    type: domain\n    source: missing.txt\n    target: direct\n"), 0o640); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if _, _, err := FindRuleset(manifest, "unknown"); err == nil || !strings.Contains(err.Error(), "unknown ruleset: unknown") {
		t.Fatalf("expected unknown ruleset error, got %v", err)
	}
	if _, _, err := FindRuleset(manifest, "test"); err == nil || !strings.Contains(err.Error(), "missing source") {
		t.Fatalf("expected missing source error, got %v", err)
	}
	if _, err := DescribeRuleset(manifest, "test"); err == nil || !strings.Contains(err.Error(), "missing source") {
		t.Fatalf("expected describe missing source error, got %v", err)
	}
}

func TestDescribeListEntriesDescribeRulesetAndRemoveEntry(t *testing.T) {
	root := filepath.Join(t.TempDir(), "rules-repo", "default")
	if err := InitDefaultRepo(root); err != nil {
		t.Fatalf("init repo: %v", err)
	}
	manifest := filepath.Join(root, "manifest.yaml")

	describe, err := Describe(manifest)
	if err != nil {
		t.Fatalf("describe repo: %v", err)
	}
	for _, needle := range []string{
		"规则仓库:",
		"- fcm-site: type=domain target=proxy entries=14 source=rules/proxy/fcm-site.txt",
		"总规则数:",
	} {
		if !strings.Contains(strings.Join(describe, "\n"), needle) {
			t.Fatalf("missing %q in describe output:\n%s", needle, strings.Join(describe, "\n"))
		}
	}

	list, err := ListEntries(manifest, "fcm-site", "google")
	if err != nil {
		t.Fatalf("list entries: %v", err)
	}
	if len(list) == 0 || !strings.Contains(list[0], "dl.google.com") {
		t.Fatalf("expected google entries, got %#v", list)
	}

	allEntries, err := ListEntries(manifest, "fcm-site", "")
	if err != nil {
		t.Fatalf("list all entries: %v", err)
	}
	if len(allEntries) < len(list) {
		t.Fatalf("expected unfiltered list to include at least the filtered rows, got %#v", allEntries)
	}

	ruleset, err := DescribeRuleset(manifest, "fcm-site")
	if err != nil {
		t.Fatalf("describe ruleset: %v", err)
	}
	for _, needle := range []string{
		"ruleset=fcm-site",
		"type=domain",
		"target=proxy",
		"source=rules/proxy/fcm-site.txt",
	} {
		if !strings.Contains(strings.Join(ruleset, "\n"), needle) {
			t.Fatalf("missing %q in ruleset description:\n%s", needle, strings.Join(ruleset, "\n"))
		}
	}

	if err := AppendEntry(manifest, "fcm-site", "codex.example.com"); err != nil {
		t.Fatalf("append codex entry: %v", err)
	}
	if err := RemoveEntry(manifest, "fcm-site", "codex.example.com"); err != nil {
		t.Fatalf("remove codex entry: %v", err)
	}
	removed, err := ListEntries(manifest, "fcm-site", "codex")
	if err != nil {
		t.Fatalf("list removed entry: %v", err)
	}
	if len(removed) != 0 {
		t.Fatalf("expected codex entry to be removed, got %#v", removed)
	}
}

func TestAppendEntryTrimsAndDeduplicatesValues(t *testing.T) {
	root := filepath.Join(t.TempDir(), "rules-repo", "default")
	if err := InitDefaultRepo(root); err != nil {
		t.Fatalf("init repo: %v", err)
	}
	manifest := filepath.Join(root, "manifest.yaml")
	if err := AppendEntry(manifest, "fcm-site", "  codex.example.com  "); err != nil {
		t.Fatalf("append trimmed entry: %v", err)
	}
	if err := AppendEntry(manifest, "fcm-site", "codex.example.com"); err != nil {
		t.Fatalf("append duplicate entry: %v", err)
	}
	lines, err := ListEntries(manifest, "fcm-site", "codex")
	if err != nil {
		t.Fatalf("list entries: %v", err)
	}
	if len(lines) != 1 || !strings.Contains(lines[0], "codex.example.com") {
		t.Fatalf("expected one trimmed deduped entry, got %#v", lines)
	}
}

func TestInitDefaultRepoIsNoopWhenManifestAlreadyExists(t *testing.T) {
	root := t.TempDir()
	manifest := filepath.Join(root, "manifest.yaml")
	sentinel := filepath.Join(root, "keep.txt")
	if err := os.WriteFile(manifest, []byte("rulesets:\n  - name: local\n    category: demo\n    type: domain\n    source: keep.txt\n    target: direct\n"), 0o640); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if err := os.WriteFile(sentinel, []byte("keep"), 0o640); err != nil {
		t.Fatalf("write sentinel: %v", err)
	}
	if err := InitDefaultRepo(root); err != nil {
		t.Fatalf("init default repo noop: %v", err)
	}
	body, err := os.ReadFile(sentinel)
	if err != nil {
		t.Fatalf("read sentinel: %v", err)
	}
	if string(body) != "keep" {
		t.Fatalf("expected sentinel to stay untouched, got %q", string(body))
	}
}

func TestInitDefaultRepoReturnsErrorWhenTargetRootIsFile(t *testing.T) {
	root := filepath.Join(t.TempDir(), "blocked")
	if err := os.WriteFile(root, []byte("blocked"), 0o640); err != nil {
		t.Fatalf("write blocking file: %v", err)
	}
	if err := InitDefaultRepo(root); err == nil || (!strings.Contains(err.Error(), "not a directory") && !strings.Contains(err.Error(), "file exists")) {
		t.Fatalf("expected init repo path error, got %v", err)
	}
}

func TestInitDefaultRepoReturnsErrorWhenManifestPathIsDirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "manifest.yaml"), 0o755); err != nil {
		t.Fatalf("mkdir blocking manifest path: %v", err)
	}
	if err := InitDefaultRepo(root); err == nil || !strings.Contains(err.Error(), "manifest path is directory") {
		t.Fatalf("expected manifest path error, got %v", err)
	}
}

func TestValidateEntrySupportsKnownRuleTypes(t *testing.T) {
	for _, tc := range []struct {
		name     string
		ruleType string
		value    string
	}{
		{name: "ip-cidr", ruleType: "ip_cidr", value: "192.168.1.0/24"},
		{name: "suffix", ruleType: "domain_suffix", value: "example.com"},
		{name: "keyword", ruleType: "domain_keyword", value: "google"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateEntry(tc.ruleType, tc.value, "entries.txt"); err != nil {
				t.Fatalf("validate entry: %v", err)
			}
		})
	}
	for _, tc := range []struct {
		name     string
		ruleType string
		value    string
	}{
		{name: "comma", ruleType: "domain", value: "bad,entry"},
		{name: "newline", ruleType: "domain", value: "bad\nentry"},
		{name: "invalid-ip-cidr", ruleType: "ip_cidr", value: "999.999.999.999/32"},
		{name: "domain-suffix-space", ruleType: "domain_suffix", value: "bad suffix"},
		{name: "keyword-space", ruleType: "domain_keyword", value: "bad keyword"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateEntry(tc.ruleType, tc.value, "entries.txt"); err == nil {
				t.Fatalf("expected invalid entry error")
			}
		})
	}
}

func TestSearchRejectsEmptyKeywordAndRenderRejectsUnsupportedEntries(t *testing.T) {
	root := filepath.Join(t.TempDir(), "rules-repo", "default")
	if err := InitDefaultRepo(root); err != nil {
		t.Fatalf("init repo: %v", err)
	}
	manifest := filepath.Join(root, "manifest.yaml")
	if _, err := Search(manifest, "   "); err == nil || !strings.Contains(err.Error(), "empty keyword") {
		t.Fatalf("expected empty keyword error, got %v", err)
	}

	dir := t.TempDir()
	manifest = filepath.Join(dir, "manifest.yaml")
	if err := os.WriteFile(manifest, []byte("rulesets:\n  - name: test\n    category: demo\n    type: domain\n    source: entries.txt\n    target: direct\n"), 0o640); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "entries.txt"), []byte("bad entry\n"), 0o640); err != nil {
		t.Fatalf("write invalid entries: %v", err)
	}
	if _, err := Render(manifest); err == nil || !strings.Contains(err.Error(), "invalid domain entry") {
		t.Fatalf("expected invalid render error, got %v", err)
	}
}

func TestSearchReportsZeroMatchesWithTrimmedKeyword(t *testing.T) {
	root := filepath.Join(t.TempDir(), "rules-repo", "default")
	if err := InitDefaultRepo(root); err != nil {
		t.Fatalf("init repo: %v", err)
	}
	manifest := filepath.Join(root, "manifest.yaml")
	lines, err := Search(manifest, "   no-such-keyword   ")
	if err != nil {
		t.Fatalf("search repo: %v", err)
	}
	if len(lines) != 2 {
		t.Fatalf("expected keyword header and zero-match footer, got %#v", lines)
	}
	if lines[0] != "keyword=no-such-keyword" {
		t.Fatalf("unexpected search header: %#v", lines[0])
	}
	if lines[1] != "matched=0" {
		t.Fatalf("unexpected search footer: %#v", lines[1])
	}
}

func TestRenderDeduplicatesAcrossRulesetsAndNormalizesManifestCase(t *testing.T) {
	dir := t.TempDir()
	manifest := filepath.Join(dir, "manifest.yaml")
	if err := os.WriteFile(manifest, []byte("rulesets:\n  - name: first\n    category: demo\n    type: DOMAIN_SUFFIX\n    source: first.txt\n    target: DIRECT\n  - name: second\n    category: demo\n    type: domain_suffix\n    source: second.txt\n    target: direct\n"), 0o640); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "first.txt"), []byte("example.com\n"), 0o640); err != nil {
		t.Fatalf("write first entries: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "second.txt"), []byte("example.com\n"), 0o640); err != nil {
		t.Fatalf("write second entries: %v", err)
	}
	lines, err := Render(manifest)
	if err != nil {
		t.Fatalf("render manifest: %v", err)
	}
	rendered := strings.Join(lines, "\n")
	if strings.Count(rendered, "DOMAIN-SUFFIX,example.com,DIRECT") != 1 {
		t.Fatalf("expected one deduped normalized rule, got %#v", lines)
	}
}

func TestQueryCommandsReturnValidationErrors(t *testing.T) {
	dir := t.TempDir()
	manifest := filepath.Join(dir, "manifest.yaml")
	if err := os.WriteFile(manifest, []byte("rulesets:\n  - name: test\n    category: demo\n    type: domain\n    source: entries.txt\n    target: direct\n"), 0o640); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "entries.txt"), []byte("bad entry\n"), 0o640); err != nil {
		t.Fatalf("write invalid entries: %v", err)
	}

	tests := []struct {
		name string
		run  func() error
	}{
		{"describe", func() error {
			_, err := Describe(manifest)
			return err
		}},
		{"list", func() error {
			_, err := ListEntries(manifest, "test", "")
			return err
		}},
		{"describe-ruleset", func() error {
			_, err := DescribeRuleset(manifest, "test")
			return err
		}},
		{"search", func() error {
			_, err := Search(manifest, "bad")
			return err
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.run(); err == nil || !strings.Contains(err.Error(), "invalid domain entry") {
				t.Fatalf("expected validation error, got %v", err)
			}
		})
	}
}

func TestRemoveCommandsPropagateEntryReadErrors(t *testing.T) {
	dir := t.TempDir()
	manifest := filepath.Join(dir, "manifest.yaml")
	if err := os.WriteFile(manifest, []byte("rulesets:\n  - name: test\n    category: demo\n    type: domain\n    source: entries.txt\n    target: direct\n"), 0o640); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "entries.txt"), []byte("example.com\nexample.com\n"), 0o640); err != nil {
		t.Fatalf("write duplicate entries: %v", err)
	}

	if err := RemoveEntry(manifest, "test", "example.com"); err == nil || !strings.Contains(err.Error(), "duplicate rule entry") {
		t.Fatalf("expected duplicate entry error from remove, got %v", err)
	}
	if err := RemoveEntryIndex(manifest, "test", 1); err == nil || !strings.Contains(err.Error(), "duplicate rule entry") {
		t.Fatalf("expected duplicate entry error from remove-index, got %v", err)
	}
}

func TestWriteEntriesReturnsErrorWhenParentPathIsBlocked(t *testing.T) {
	root := t.TempDir()
	blockedDir := filepath.Join(root, "blocked")
	if err := os.WriteFile(blockedDir, []byte("blocked"), 0o640); err != nil {
		t.Fatalf("write blocking file: %v", err)
	}
	if err := writeEntries(filepath.Join(blockedDir, "entries.txt"), []string{"example.com"}); err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("expected blocked path error, got %v", err)
	}
}

func TestAppendEntryRejectsInvalidEntryAndRemoveEntryNoopsOnMissingValue(t *testing.T) {
	dir := t.TempDir()
	manifest := filepath.Join(dir, "manifest.yaml")
	source := filepath.Join(dir, "entries.txt")
	if err := os.WriteFile(manifest, []byte("rulesets:\n  - name: test\n    category: demo\n    type: domain\n    source: entries.txt\n    target: direct\n"), 0o640); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if err := os.WriteFile(source, []byte("example.com\n"), 0o640); err != nil {
		t.Fatalf("write source: %v", err)
	}
	before, err := os.ReadFile(source)
	if err != nil {
		t.Fatalf("read source before: %v", err)
	}
	if err := AppendEntry(manifest, "test", "bad entry"); err == nil || !strings.Contains(err.Error(), "invalid domain entry") {
		t.Fatalf("expected invalid append error, got %v", err)
	}
	if err := RemoveEntry(manifest, "test", "missing.example"); err != nil {
		t.Fatalf("remove missing entry: %v", err)
	}
	after, err := os.ReadFile(source)
	if err != nil {
		t.Fatalf("read source after: %v", err)
	}
	if string(before) != string(after) {
		t.Fatalf("expected missing value removal to be a no-op, before=%q after=%q", string(before), string(after))
	}
}

func TestAppendEntryPropagatesSourceReadErrorWhenSourceIsDirectory(t *testing.T) {
	dir := t.TempDir()
	manifest := filepath.Join(dir, "manifest.yaml")
	source := filepath.Join(dir, "entries.txt")
	if err := os.WriteFile(manifest, []byte("rulesets:\n  - name: test\n    category: demo\n    type: domain\n    source: entries.txt\n    target: direct\n"), 0o640); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatalf("mkdir source path: %v", err)
	}
	if err := AppendEntry(manifest, "test", "example.com"); err == nil || !strings.Contains(err.Error(), "is a directory") {
		t.Fatalf("expected source read error, got %v", err)
	}
}

func TestAppendAndRemoveEntryIndexDeduplicateAndRewrite(t *testing.T) {
	dir := t.TempDir()
	manifest := filepath.Join(dir, "manifest.yaml")
	source := filepath.Join(dir, "entries.txt")
	if err := os.WriteFile(manifest, []byte("rulesets:\n  - name: test\n    category: demo\n    type: domain\n    source: entries.txt\n    target: direct\n"), 0o640); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if err := os.WriteFile(source, nil, 0o640); err != nil {
		t.Fatalf("write empty source: %v", err)
	}
	if err := AppendEntry(manifest, "test", "example.com"); err != nil {
		t.Fatalf("append entry: %v", err)
	}
	if err := AppendEntry(manifest, "test", "example.com"); err != nil {
		t.Fatalf("append duplicate entry: %v", err)
	}
	lines, err := ReadEntries(source)
	if err != nil {
		t.Fatalf("read entries: %v", err)
	}
	if len(lines) != 1 || lines[0] != "example.com" {
		t.Fatalf("unexpected entries after append: %#v", lines)
	}
	if err := RemoveEntryIndex(manifest, "test", 1); err != nil {
		t.Fatalf("remove entry index: %v", err)
	}
	lines, err = ReadEntries(source)
	if err != nil {
		t.Fatalf("read entries after remove: %v", err)
	}
	if len(lines) != 0 {
		t.Fatalf("expected empty entries after remove, got %#v", lines)
	}
}

func TestAppendAndRemoveEntryTrimValues(t *testing.T) {
	dir := t.TempDir()
	manifest := filepath.Join(dir, "manifest.yaml")
	source := filepath.Join(dir, "entries.txt")
	if err := os.WriteFile(manifest, []byte("rulesets:\n  - name: test\n    category: demo\n    type: domain\n    source: entries.txt\n    target: direct\n"), 0o640); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if err := os.WriteFile(source, nil, 0o640); err != nil {
		t.Fatalf("write source: %v", err)
	}
	if err := AppendEntry(manifest, "test", " example.com "); err != nil {
		t.Fatalf("append trimmed entry: %v", err)
	}
	if err := AppendEntry(manifest, "test", "example.com"); err != nil {
		t.Fatalf("append duplicate entry: %v", err)
	}
	lines, err := ReadEntries(source)
	if err != nil {
		t.Fatalf("read entries: %v", err)
	}
	if len(lines) != 1 || lines[0] != "example.com" {
		t.Fatalf("expected one trimmed entry, got %#v", lines)
	}
	if err := RemoveEntry(manifest, "test", " example.com "); err != nil {
		t.Fatalf("remove trimmed entry: %v", err)
	}
	lines, err = ReadEntries(source)
	if err != nil {
		t.Fatalf("read entries after remove: %v", err)
	}
	if len(lines) != 0 {
		t.Fatalf("expected trimmed removal to delete entry, got %#v", lines)
	}
}

func TestRemoveEntryIndexRejectsOutOfRange(t *testing.T) {
	dir := t.TempDir()
	manifest := filepath.Join(dir, "manifest.yaml")
	source := filepath.Join(dir, "entries.txt")
	if err := os.WriteFile(manifest, []byte("rulesets:\n  - name: test\n    category: demo\n    type: domain\n    source: entries.txt\n    target: direct\n"), 0o640); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if err := os.WriteFile(source, []byte("example.com\n"), 0o640); err != nil {
		t.Fatalf("write source: %v", err)
	}
	if err := RemoveEntryIndex(manifest, "test", 0); err == nil || !strings.Contains(err.Error(), "entry index out of range") {
		t.Fatalf("expected out of range error, got %v", err)
	}
	if err := RemoveEntryIndex(manifest, "test", 2); err == nil || !strings.Contains(err.Error(), "entry index out of range") {
		t.Fatalf("expected out of range error, got %v", err)
	}
}
