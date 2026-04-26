package rulesrepo

import (
	"embed"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	yaml "gopkg.in/yaml.v2"
)

//go:embed assets/**
var embedded embed.FS

type Manifest struct {
	Rulesets []Ruleset `yaml:"rulesets"`
}

type Ruleset struct {
	Name     string `yaml:"name"`
	Category string `yaml:"category"`
	Type     string `yaml:"type"`
	Source   string `yaml:"source"`
	Target   string `yaml:"target"`
	Note     string `yaml:"note"`
}

var domainPattern = regexp.MustCompile(`^[A-Za-z0-9._*-]+$`)

func InitDefaultRepo(targetRoot string) error {
	if _, err := os.Stat(filepath.Join(targetRoot, "manifest.yaml")); err == nil {
		return nil
	}
	entries, err := embedded.ReadDir("assets/default")
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := copyTree("assets/default/"+entry.Name(), filepath.Join(targetRoot, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func copyTree(src, dst string) error {
	info, err := embedded.ReadDir(src)
	if err == nil {
		if err := os.MkdirAll(dst, 0o755); err != nil {
			return err
		}
		for _, child := range info {
			if err := copyTree(filepath.Join(src, child.Name()), filepath.Join(dst, child.Name())); err != nil {
				return err
			}
		}
		return nil
	}
	raw, err := embedded.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, raw, 0o644)
}

func LoadManifest(path string) (Manifest, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, err
	}
	var m Manifest
	if err := yaml.Unmarshal(raw, &m); err != nil {
		return Manifest{}, fmt.Errorf("parse manifest: %w", err)
	}
	if len(m.Rulesets) == 0 {
		return Manifest{}, fmt.Errorf("empty manifest: %s", path)
	}
	return m, nil
}

func FindRuleset(manifestPath, name string) (Ruleset, string, error) {
	m, err := LoadManifest(manifestPath)
	if err != nil {
		return Ruleset{}, "", err
	}
	root := filepath.Dir(manifestPath)
	for _, item := range m.Rulesets {
		if item.Name == name {
			source := filepath.Join(root, item.Source)
			if _, err := os.Stat(source); err != nil {
				return Ruleset{}, "", fmt.Errorf("missing source: %s", source)
			}
			return item, source, nil
		}
	}
	return Ruleset{}, "", fmt.Errorf("unknown ruleset: %s", name)
}

func ReadEntries(path string) ([]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	lines := []string{}
	seen := map[string]struct{}{}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if _, ok := seen[line]; ok {
			return nil, fmt.Errorf("duplicate rule entry in %s: %s", path, line)
		}
		seen[line] = struct{}{}
		lines = append(lines, line)
	}
	return lines, nil
}

func ValidateEntry(ruleType, value, source string) error {
	if value == "" || strings.ContainsAny(value, ",\n\r") {
		return fmt.Errorf("invalid %s entry in %s: %s", ruleType, source, value)
	}
	switch ruleType {
	case "ip_cidr":
		if _, err := netip.ParsePrefix(value); err != nil {
			return fmt.Errorf("invalid ip_cidr entry in %s: %s", source, value)
		}
	case "domain", "domain_suffix":
		if strings.ContainsAny(value, " \t") || !domainPattern.MatchString(value) {
			return fmt.Errorf("invalid %s entry in %s: %s", ruleType, source, value)
		}
	case "domain_keyword":
		if strings.ContainsAny(value, " \t") {
			return fmt.Errorf("invalid domain_keyword entry in %s: %s", source, value)
		}
	default:
		return fmt.Errorf("unsupported rule type: %s", ruleType)
	}
	return nil
}

func Render(manifestPath string) ([]string, error) {
	m, err := LoadManifest(manifestPath)
	if err != nil {
		return nil, err
	}
	root := filepath.Dir(manifestPath)
	lines := []string{}
	seen := map[string]struct{}{}
	for _, item := range m.Rulesets {
		kind, ok := map[string]string{
			"domain":         "DOMAIN",
			"domain_suffix":  "DOMAIN-SUFFIX",
			"domain_keyword": "DOMAIN-KEYWORD",
			"ip_cidr":        "IP-CIDR",
		}[strings.ToLower(item.Type)]
		if !ok {
			return nil, fmt.Errorf("unsupported rule type in manifest: %s", item.Type)
		}
		target, ok := map[string]string{
			"direct": "DIRECT",
			"proxy":  "PROXY",
			"reject": "REJECT",
			"auto":   "AUTO",
		}[strings.ToLower(item.Target)]
		if !ok {
			return nil, fmt.Errorf("unsupported rule target in manifest: %s", item.Target)
		}
		entries, err := ReadEntries(filepath.Join(root, item.Source))
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if err := ValidateEntry(strings.ToLower(item.Type), entry, item.Source); err != nil {
				return nil, err
			}
			line := fmt.Sprintf("%s,%s,%s", kind, entry, target)
			if _, ok := seen[line]; ok {
				continue
			}
			seen[line] = struct{}{}
			lines = append(lines, line)
		}
	}
	return lines, nil
}

func Describe(manifestPath string) ([]string, error) {
	m, err := LoadManifest(manifestPath)
	if err != nil {
		return nil, err
	}
	root := filepath.Dir(manifestPath)
	lines := []string{fmt.Sprintf("规则仓库: %s", root)}
	total := 0
	for _, item := range m.Rulesets {
		entries, err := ReadEntries(filepath.Join(root, item.Source))
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if err := ValidateEntry(strings.ToLower(item.Type), entry, item.Source); err != nil {
				return nil, err
			}
		}
		total += len(entries)
		lines = append(lines, fmt.Sprintf("- %s: type=%s target=%s entries=%d source=%s", item.Name, item.Type, item.Target, len(entries), item.Source))
	}
	lines = append(lines, fmt.Sprintf("总规则数: %d", total))
	return lines, nil
}

func Search(manifestPath, keyword string) ([]string, error) {
	m, err := LoadManifest(manifestPath)
	if err != nil {
		return nil, err
	}
	root := filepath.Dir(manifestPath)
	needle := strings.ToLower(strings.TrimSpace(keyword))
	if needle == "" {
		return nil, fmt.Errorf("empty keyword")
	}
	lines := []string{fmt.Sprintf("keyword=%s", strings.TrimSpace(keyword))}
	matched := 0
	for _, item := range m.Rulesets {
		entries, err := ReadEntries(filepath.Join(root, item.Source))
		if err != nil {
			return nil, err
		}
		for idx, entry := range entries {
			if err := ValidateEntry(strings.ToLower(item.Type), entry, item.Source); err != nil {
				return nil, err
			}
			if !strings.Contains(strings.ToLower(entry), needle) {
				continue
			}
			matched++
			lines = append(lines, fmt.Sprintf("%d\t%s\t%s\t%s\t%d\t%s", matched, item.Name, item.Type, item.Target, idx+1, entry))
		}
	}
	lines = append(lines, fmt.Sprintf("matched=%d", matched))
	return lines, nil
}

func ListEntries(manifestPath, rulesetName, keyword string) ([]string, error) {
	ruleset, source, err := FindRuleset(manifestPath, rulesetName)
	if err != nil {
		return nil, err
	}
	entries, err := ReadEntries(source)
	if err != nil {
		return nil, err
	}
	needle := strings.ToLower(strings.TrimSpace(keyword))
	lines := []string{}
	for idx, entry := range entries {
		if err := ValidateEntry(strings.ToLower(ruleset.Type), entry, source); err != nil {
			return nil, err
		}
		if needle != "" && !strings.Contains(strings.ToLower(entry), needle) {
			continue
		}
		lines = append(lines, fmt.Sprintf("%d\t%s", idx+1, entry))
	}
	return lines, nil
}

func DescribeRuleset(manifestPath, rulesetName string) ([]string, error) {
	ruleset, source, err := FindRuleset(manifestPath, rulesetName)
	if err != nil {
		return nil, err
	}
	entries, err := ReadEntries(source)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if err := ValidateEntry(strings.ToLower(ruleset.Type), entry, source); err != nil {
			return nil, err
		}
	}
	return []string{
		fmt.Sprintf("ruleset=%s", ruleset.Name),
		fmt.Sprintf("type=%s", ruleset.Type),
		fmt.Sprintf("target=%s", ruleset.Target),
		fmt.Sprintf("entries=%d", len(entries)),
		fmt.Sprintf("source=%s", ruleset.Source),
	}, nil
}

func AppendEntry(manifestPath, rulesetName, value string) error {
	ruleset, source, err := FindRuleset(manifestPath, rulesetName)
	if err != nil {
		return err
	}
	if err := ValidateEntry(strings.ToLower(ruleset.Type), value, source); err != nil {
		return err
	}
	entries, err := ReadEntries(source)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if slices.Contains(entries, value) {
		return nil
	}
	entries = append(entries, value)
	return writeEntries(source, entries)
}

func RemoveEntry(manifestPath, rulesetName, value string) error {
	_, source, err := FindRuleset(manifestPath, rulesetName)
	if err != nil {
		return err
	}
	entries, err := ReadEntries(source)
	if err != nil {
		return err
	}
	filtered := entries[:0]
	for _, entry := range entries {
		if entry != value {
			filtered = append(filtered, entry)
		}
	}
	return writeEntries(source, filtered)
}

func RemoveEntryIndex(manifestPath, rulesetName string, index int) error {
	_, source, err := FindRuleset(manifestPath, rulesetName)
	if err != nil {
		return err
	}
	entries, err := ReadEntries(source)
	if err != nil {
		return err
	}
	if index < 1 || index > len(entries) {
		return fmt.Errorf("entry index out of range")
	}
	entries = append(entries[:index-1], entries[index:]...)
	return writeEntries(source, entries)
}

func writeEntries(path string, entries []string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	body := ""
	if len(entries) > 0 {
		body = strings.Join(entries, "\n") + "\n"
	}
	return os.WriteFile(path, []byte(body), 0o644)
}
