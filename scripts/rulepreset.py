#!/usr/bin/env python3

import argparse
import ipaddress
import re
from pathlib import Path


RULE_KIND_MAP = {
    "domain": "DOMAIN",
    "domain_suffix": "DOMAIN-SUFFIX",
    "domain_keyword": "DOMAIN-KEYWORD",
    "ip_cidr": "IP-CIDR",
}

RULE_TARGET_MAP = {
    "direct": "DIRECT",
    "proxy": "PROXY",
    "reject": "REJECT",
    "auto": "AUTO",
}


def fail(message: str) -> None:
    raise SystemExit(message)


def parse_manifest(path: Path) -> list[dict[str, str]]:
    items: list[dict[str, str]] = []
    current: dict[str, str] | None = None

    for raw in path.read_text(encoding="utf-8").splitlines():
        line = raw.strip()
        if not line or line.startswith("#") or line == "rulesets:":
            continue
        if line.startswith("- name:"):
            if current:
                items.append(current)
            current = {"name": line.split(":", 1)[1].strip()}
            continue
        if current is None or ":" not in line:
            continue
        key, value = [part.strip() for part in line.split(":", 1)]
        current[key] = value.strip('"')

    if current:
        items.append(current)

    if not items:
        fail(f"empty preset manifest: {path}")
    return items


def find_ruleset(path: Path, ruleset_name: str) -> tuple[dict[str, str], Path]:
    root = path.parent
    for item in parse_manifest(path):
        if item.get("name") == ruleset_name:
            source_rel = item.get("source", "")
            if not source_rel:
                fail(f"missing source in preset manifest: {path}")
            source = root / source_rel
            if not source.exists():
                fail(f"missing source file: {source}")
            return item, source
    fail(f"unknown ruleset: {ruleset_name}")


def read_source_lines(path: Path) -> list[str]:
    lines: list[str] = []
    seen: set[str] = set()

    for raw in path.read_text(encoding="utf-8").splitlines():
        line = raw.strip()
        if not line or line.startswith("#"):
            continue
        if line in seen:
            fail(f"duplicate rule entry in {path}: {line}")
        seen.add(line)
        lines.append(line)

    return lines


def validate_rule_entry(rule_type: str, value: str, source: Path) -> None:
    if not value:
        fail(f"empty {rule_type} entry in {source}")
    if "," in value or "\n" in value or "\r" in value:
        fail(f"invalid {rule_type} entry in {source}: {value}")

    if rule_type == "ip_cidr":
        if "/" not in value:
            fail(f"invalid ip_cidr entry in {source}: {value}")
        try:
            ipaddress.ip_network(value, strict=False)
        except ValueError:
            fail(f"invalid ip_cidr entry in {source}: {value}")
        return

    if rule_type in {"domain", "domain_suffix"}:
        if any(ch.isspace() for ch in value):
            fail(f"invalid {rule_type} entry in {source}: {value}")
        if not re.fullmatch(r"[A-Za-z0-9._*-]+", value):
            fail(f"invalid {rule_type} entry in {source}: {value}")
        return

    if rule_type == "domain_keyword":
        if any(ch.isspace() for ch in value):
            fail(f"invalid domain_keyword entry in {source}: {value}")
        return

    fail(f"unsupported rule type: {rule_type}")


def write_source_lines(path: Path, lines: list[str]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(("\n".join(lines) + "\n") if lines else "", encoding="utf-8")


def render_manifest(path: Path) -> list[str]:
    manifest_items = parse_manifest(path)
    root = path.parent
    rendered: list[str] = []
    seen: set[str] = set()

    for item in manifest_items:
        source_rel = item.get("source", "")
        rule_type = item.get("type", "").lower()
        target = item.get("target", "").lower()
        if not source_rel:
            fail(f"missing source in preset manifest: {path}")
        if rule_type not in RULE_KIND_MAP:
            fail(f"unsupported rule type in {path}: {rule_type}")
        if target not in RULE_TARGET_MAP:
            fail(f"unsupported rule target in {path}: {target}")

        source = root / source_rel
        if not source.exists():
            fail(f"missing source file: {source}")

        rule_kind = RULE_KIND_MAP[rule_type]
        rule_target = RULE_TARGET_MAP[target]

        for value in read_source_lines(source):
            validate_rule_entry(rule_type, value, source)
            line = f"{rule_kind},{value},{rule_target}"
            if line in seen:
                continue
            seen.add(line)
            rendered.append(line)

    return rendered


def describe_manifest(path: Path) -> list[str]:
    manifest_items = parse_manifest(path)
    root = path.parent
    lines = [f"规则仓库: {root}"]

    total = 0
    for item in manifest_items:
        source_rel = item.get("source", "")
        source = root / source_rel
        entries = read_source_lines(source)
        rule_type = item.get("type", "").lower()
        for entry in entries:
            validate_rule_entry(rule_type, entry, source)
        total += len(entries)
        lines.append(
            f"- {item['name']}: type={item.get('type', '')} target={item.get('target', '')} entries={len(entries)} source={source_rel}"
        )

    lines.append(f"总规则数: {total}")
    return lines


def describe_ruleset(path: Path, ruleset_name: str) -> list[str]:
    ruleset, source = find_ruleset(path, ruleset_name)
    entries = read_source_lines(source)
    rule_type = ruleset.get("type", "").lower()
    for entry in entries:
        validate_rule_entry(rule_type, entry, source)

    return [
        f"ruleset={ruleset['name']}",
        f"type={ruleset.get('type', '')}",
        f"target={ruleset.get('target', '')}",
        f"entries={len(entries)}",
        f"source={ruleset.get('source', '')}",
    ]


def search_entries(path: Path, keyword: str) -> list[str]:
    manifest_items = parse_manifest(path)
    root = path.parent
    needle = keyword.strip().lower()
    if not needle:
        fail("empty keyword")

    lines = [f"keyword={keyword.strip()}"]
    matched = 0
    for item in manifest_items:
        source_rel = item.get("source", "")
        source = root / source_rel
        entries = read_source_lines(source)
        rule_type = item.get("type", "").lower()
        for idx, entry in enumerate(entries, start=1):
            validate_rule_entry(rule_type, entry, source)
            if needle not in entry.lower():
                continue
            lines.append(
                f"{matched + 1}\t{item.get('name', '')}\t{item.get('type', '')}\t{item.get('target', '')}\t{idx}\t{entry}"
            )
            matched += 1

    lines.append(f"matched={matched}")
    return lines


def cmd_render(args: argparse.Namespace) -> int:
    manifest = Path(args.manifest_file)
    output = Path(args.output_file)
    lines = render_manifest(manifest)
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(("\n".join(lines) + "\n") if lines else "", encoding="utf-8")
    return 0


def cmd_summary(args: argparse.Namespace) -> int:
    manifest = Path(args.manifest_file)
    lines = render_manifest(manifest)
    print(f"rules={len(lines)}")
    return 0


def cmd_describe(args: argparse.Namespace) -> int:
    manifest = Path(args.manifest_file)
    for line in describe_manifest(manifest):
        print(line)
    return 0


def cmd_list_entries(args: argparse.Namespace) -> int:
    manifest = Path(args.manifest_file)
    ruleset, source = find_ruleset(manifest, args.ruleset_name)
    entries = read_source_lines(source)
    keyword = (args.keyword or "").strip().lower()
    for line in describe_ruleset(manifest, args.ruleset_name):
        print(line)
    matched = 0
    for idx, entry in enumerate(entries, start=1):
        if keyword and keyword not in entry.lower():
            continue
        print(f"{idx}\t{entry}")
        matched += 1
    if keyword:
        print(f"matched={matched}")
    return 0


def cmd_describe_ruleset(args: argparse.Namespace) -> int:
    manifest = Path(args.manifest_file)
    for line in describe_ruleset(manifest, args.ruleset_name):
        print(line)
    return 0


def cmd_search_entries(args: argparse.Namespace) -> int:
    manifest = Path(args.manifest_file)
    for line in search_entries(manifest, args.keyword):
        print(line)
    return 0


def cmd_append_entry(args: argparse.Namespace) -> int:
    manifest = Path(args.manifest_file)
    ruleset, source = find_ruleset(manifest, args.ruleset_name)
    value = args.value.strip()
    if not value:
        fail("empty rule entry")
    validate_rule_entry(ruleset.get("type", "").lower(), value, source)
    entries = read_source_lines(source)
    if value in entries:
        fail(f"duplicate rule entry in {source}: {value}")
    entries.append(value)
    write_source_lines(source, entries)
    return 0


def cmd_remove_entry(args: argparse.Namespace) -> int:
    manifest = Path(args.manifest_file)
    _ruleset, source = find_ruleset(manifest, args.ruleset_name)
    value = args.value.strip()
    if not value:
        fail("empty rule entry")
    entries = read_source_lines(source)
    if value not in entries:
        fail(f"rule entry not found in {source}: {value}")
    entries = [entry for entry in entries if entry != value]
    write_source_lines(source, entries)
    return 0


def cmd_remove_entry_index(args: argparse.Namespace) -> int:
    manifest = Path(args.manifest_file)
    _ruleset, source = find_ruleset(manifest, args.ruleset_name)
    entries = read_source_lines(source)
    index = int(args.index)
    if index < 1 or index > len(entries):
        fail(f"rule entry index out of range in {source}: {index}")
    del entries[index - 1]
    write_source_lines(source, entries)
    return 0


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser()
    sub = parser.add_subparsers(dest="command", required=True)

    render = sub.add_parser("render")
    render.add_argument("manifest_file")
    render.add_argument("output_file")
    render.set_defaults(func=cmd_render)

    summary = sub.add_parser("summary")
    summary.add_argument("manifest_file")
    summary.set_defaults(func=cmd_summary)

    describe = sub.add_parser("describe")
    describe.add_argument("manifest_file")
    describe.set_defaults(func=cmd_describe)

    describe_ruleset = sub.add_parser("describe-ruleset")
    describe_ruleset.add_argument("manifest_file")
    describe_ruleset.add_argument("ruleset_name")
    describe_ruleset.set_defaults(func=cmd_describe_ruleset)

    search_entries_cmd = sub.add_parser("search-entries")
    search_entries_cmd.add_argument("manifest_file")
    search_entries_cmd.add_argument("keyword")
    search_entries_cmd.set_defaults(func=cmd_search_entries)

    list_entries = sub.add_parser("list-entries")
    list_entries.add_argument("manifest_file")
    list_entries.add_argument("ruleset_name")
    list_entries.add_argument("keyword", nargs="?")
    list_entries.set_defaults(func=cmd_list_entries)

    append_entry = sub.add_parser("append-entry")
    append_entry.add_argument("manifest_file")
    append_entry.add_argument("ruleset_name")
    append_entry.add_argument("value")
    append_entry.set_defaults(func=cmd_append_entry)

    remove_entry = sub.add_parser("remove-entry")
    remove_entry.add_argument("manifest_file")
    remove_entry.add_argument("ruleset_name")
    remove_entry.add_argument("value")
    remove_entry.set_defaults(func=cmd_remove_entry)

    remove_entry_index = sub.add_parser("remove-entry-index")
    remove_entry_index.add_argument("manifest_file")
    remove_entry_index.add_argument("ruleset_name")
    remove_entry_index.add_argument("index")
    remove_entry_index.set_defaults(func=cmd_remove_entry_index)

    return parser


def main() -> int:
    parser = build_parser()
    args = parser.parse_args()
    return args.func(args)


if __name__ == "__main__":
    raise SystemExit(main())
