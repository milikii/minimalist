#!/usr/bin/env python3

import argparse
import base64
import json
import sys
import urllib.parse
import uuid
from datetime import datetime, timezone
from pathlib import Path


NODE_VERSION = 2
RULE_VERSION = 2
SUBSCRIPTION_VERSION = 1
DISABLED_PREFIX = "#DISABLED#"
SUPPORTED_SCHEMES = ("vless", "vmess", "trojan", "ss")
RULE_KIND_MAP = {
    "domain": "DOMAIN",
    "suffix": "DOMAIN-SUFFIX",
    "keyword": "DOMAIN-KEYWORD",
    "src-cidr": "SRC-IP-CIDR",
    "ip-cidr": "IP-CIDR",
    "port": "DST-PORT",
    "geoip": "GEOIP",
    "geosite": "GEOSITE",
    "ruleset": "RULE-SET",
}


def ensure_parent(path: Path) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)


def now_iso() -> str:
    return datetime.now(timezone.utc).isoformat()


def fail(message: str) -> None:
    raise SystemExit(message)


def deep_copy_json(data: dict) -> dict:
    return json.loads(json.dumps(data))


def normalize_uri(uri: str) -> str:
    return uri.strip()


def b64decode_padded(text: str) -> bytes:
    raw = text.strip()
    raw += "=" * (-len(raw) % 4)
    return base64.b64decode(raw)


def decode_subscription_lines(text: str) -> list[str]:
    raw_lines = [line.strip() for line in text.splitlines() if line.strip()]
    if any("://" in line for line in raw_lines):
        return raw_lines
    collapsed = "".join(raw_lines)
    if not collapsed:
        return []
    try:
        decoded = b64decode_padded(collapsed).decode("utf-8", errors="ignore")
    except Exception:
        return raw_lines
    return [line.strip() for line in decoded.splitlines() if line.strip()]


def object_value(mapping: dict | None, *names: str):
    if not isinstance(mapping, dict):
        return None
    for name in names:
        if name in mapping:
            return mapping[name]
    return None


def string_value(mapping: dict | None, *names: str) -> str:
    value = object_value(mapping, *names)
    if value is None:
        return ""
    if isinstance(value, str):
        return value
    return str(value)


def int_value(value) -> int | None:
    if value in (None, ""):
        return None
    try:
        return int(value)
    except (TypeError, ValueError):
        return None


def query_value(query: dict[str, list[str]], *names: str) -> str:
    for name in names:
        values = query.get(name)
        if values:
            return values[0]
    return ""


def has_query_key(query: dict[str, list[str]], *names: str) -> bool:
    return any(name in query for name in names)


def is_truthy(value: str) -> bool:
    return value.strip().lower() in {"1", "true", "yes", "on"}


def split_csv(value: str) -> list[str]:
    return [item.strip() for item in value.split(",") if item.strip()]


def query_json_object(query: dict[str, list[str]], *names: str) -> dict:
    raw = query_value(query, *names)
    if not raw:
        return {}
    try:
        value = json.loads(raw)
    except json.JSONDecodeError:
        return {}
    return value if isinstance(value, dict) else {}


def yaml_scalar(value) -> str:
    if isinstance(value, bool):
        return "true" if value else "false"
    if isinstance(value, (int, float)):
        return str(value)
    if value is None:
        return "null"
    return json.dumps(str(value), ensure_ascii=False)


def append_yaml_lines(lines: list[str], value, indent: int) -> None:
    prefix = " " * indent
    if isinstance(value, dict):
        for key, item in value.items():
            if isinstance(item, dict):
                if item:
                    lines.append(f"{prefix}{key}:")
                    append_yaml_lines(lines, item, indent + 2)
                else:
                    lines.append(f"{prefix}{key}: {{}}")
            elif isinstance(item, list):
                if item:
                    lines.append(f"{prefix}{key}:")
                    append_yaml_lines(lines, item, indent + 2)
                else:
                    lines.append(f"{prefix}{key}: []")
            else:
                lines.append(f"{prefix}{key}: {yaml_scalar(item)}")
        return

    if isinstance(value, list):
        for item in value:
            if isinstance(item, dict):
                lines.append(f"{prefix}-")
                append_yaml_lines(lines, item, indent + 2)
            elif isinstance(item, list):
                lines.append(f"{prefix}-")
                append_yaml_lines(lines, item, indent + 2)
            else:
                lines.append(f"{prefix}- {yaml_scalar(item)}")
        return

    lines.append(f"{prefix}{yaml_scalar(value)}")


def load_json(path: Path, fallback: dict) -> dict:
    if not path.exists():
        return deep_copy_json(fallback)
    data = json.loads(path.read_text(encoding="utf-8"))
    if isinstance(data, dict):
        return data
    fail(f"invalid json state: {path}")


def save_json(path: Path, data: dict) -> None:
    ensure_parent(path)
    path.write_text(json.dumps(data, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


def empty_nodes_state() -> dict:
    return {"version": NODE_VERSION, "nodes": []}


def empty_rules_state() -> dict:
    return {"version": RULE_VERSION, "rules": []}


def empty_subscriptions_state() -> dict:
    return {"version": SUBSCRIPTION_VERSION, "subscriptions": []}


def normalize_rule_kind(kind: str) -> str:
    lowered = kind.strip().lower()
    aliases = {
        "source": "src-cidr",
        "src": "src-cidr",
        "src-ip-cidr": "src-cidr",
        "dst": "ip-cidr",
        "dst-cidr": "ip-cidr",
        "ip": "ip-cidr",
        "ip-cidr": "ip-cidr",
        "dst-port": "port",
        "port": "port",
        "domain": "domain",
        "suffix": "suffix",
        "keyword": "keyword",
        "geoip": "geoip",
        "geosite": "geosite",
        "ruleset": "ruleset",
        "rule-set": "ruleset",
    }
    normalized = aliases.get(lowered, lowered)
    if normalized not in RULE_KIND_MAP:
        fail(f"unsupported rule kind: {kind}")
    return normalized


def legacy_rule_kind(kind: str) -> str | None:
    legacy = {
        "DOMAIN": "domain",
        "DOMAIN-SUFFIX": "suffix",
        "DOMAIN-KEYWORD": "keyword",
        "SRC-IP-CIDR": "src-cidr",
        "IP-CIDR": "ip-cidr",
        "DST-PORT": "port",
        "GEOIP": "geoip",
        "GEOSITE": "geosite",
        "RULE-SET": "ruleset",
    }
    return legacy.get(kind.upper())


def migrate_nodes_from_legacy(legacy_path: Path) -> dict:
    state = empty_nodes_state()
    if not legacy_path.exists():
        return state
    for raw in legacy_path.read_text(encoding="utf-8").splitlines():
        line = raw.strip()
        if not line:
            continue
        enabled = True
        if line.startswith(DISABLED_PREFIX):
            enabled = False
            line = line[len(DISABLED_PREFIX):].strip()
        if not line or (line.startswith("#") and not line.startswith(DISABLED_PREFIX)) or "://" not in line:
            continue
        state["nodes"].append(
            {
                "id": str(uuid.uuid4()),
                "name": guess_name(line),
                "enabled": enabled,
                "uri": normalize_uri(line),
                "imported_at": now_iso(),
                "source": {"kind": "legacy"},
            }
        )
    return state


def migrate_rules_from_legacy(legacy_path: Path) -> dict:
    state = empty_rules_state()
    if not legacy_path.exists():
        return state
    for raw in legacy_path.read_text(encoding="utf-8").splitlines():
        line = raw.strip()
        if not line or line.startswith("#"):
            continue
        parts = [part.strip() for part in line.split(",")]
        if len(parts) != 3:
            continue
        kind = legacy_rule_kind(parts[0])
        if not kind:
            continue
        state["rules"].append(
            {
                "id": str(uuid.uuid4()),
                "kind": kind,
                "pattern": parts[1],
                "target": parts[2],
            }
        )
    return state


def ensure_nodes_state(path: Path, legacy_path: Path | None = None) -> dict:
    if path.exists():
        return load_json(path, empty_nodes_state())
    state = migrate_nodes_from_legacy(legacy_path) if legacy_path else empty_nodes_state()
    save_json(path, state)
    return state


def ensure_rules_state(path: Path, legacy_path: Path | None = None) -> dict:
    if path.exists():
        return load_json(path, empty_rules_state())
    state = migrate_rules_from_legacy(legacy_path) if legacy_path else empty_rules_state()
    save_json(path, state)
    return state


def ensure_subscriptions_state(path: Path) -> dict:
    if path.exists():
        return load_json(path, empty_subscriptions_state())
    state = empty_subscriptions_state()
    save_json(path, state)
    return state


def ensure_unique_name(nodes: list[dict], name: str, ignore_id: str | None = None) -> None:
    for node in nodes:
        if ignore_id and node.get("id") == ignore_id:
            continue
        if node.get("name") == name:
            fail(f"duplicate node name: {name}")


def resolve_unique_name(nodes: list[dict], preferred: str) -> str:
    name = preferred
    suffix = 2
    existing = {node.get("name") for node in nodes}
    while name in existing:
        name = f"{preferred}-{suffix}"
        suffix += 1
    return name


def iter_enabled_names(nodes: list[dict]) -> list[str]:
    return [node["name"] for node in nodes if node.get("enabled")]


def parse_json_from_uri_payload(uri: str) -> dict:
    payload = normalize_uri(uri)[len("vmess://") :]
    if "#" in payload:
        payload = payload.split("#", 1)[0]
    if "?" in payload:
        payload = payload.split("?", 1)[0]
    try:
        raw = b64decode_padded(payload).decode("utf-8", errors="ignore")
        value = json.loads(raw)
    except Exception:
        fail("invalid vmess payload")
    if not isinstance(value, dict):
        fail("invalid vmess payload")
    return value


def parse_vless_uri(uri: str) -> dict:
    parsed = urllib.parse.urlsplit(normalize_uri(uri))
    query = urllib.parse.parse_qs(parsed.query, keep_blank_values=True)
    extra = query_json_object(query, "extra")
    download_settings = object_value(extra, "downloadSettings", "download-settings")
    server = parsed.hostname or ""
    port = parsed.port
    uuid_value = urllib.parse.unquote(parsed.username or "")
    if not server or port is None or not uuid_value:
        fail("invalid vless uri")
    return {
        "scheme": "vless",
        "server": server,
        "port": port,
        "uuid": uuid_value,
        "network": (query_value(query, "type") or "tcp").lower(),
        "flow": query_value(query, "flow"),
        "packet_encoding": query_value(query, "packetEncoding", "packet-encoding"),
        "security": query_value(query, "security").lower(),
        "alpn": split_csv(query_value(query, "alpn")),
        "servername": query_value(query, "sni", "servername", "serverName"),
        "fingerprint": query_value(query, "fp", "fingerprint", "client-fingerprint"),
        "encryption": query_value(query, "encryption"),
        "skip_cert_verify": is_truthy(query_value(query, "insecure", "allowInsecure", "skip-cert-verify"))
        if has_query_key(query, "insecure", "allowInsecure", "skip-cert-verify")
        else None,
        "reality_opts": reality_opts_from_query(query),
        "path": query_value(query, "path"),
        "host": query_value(query, "host"),
        "mode": query_value(query, "mode"),
        "service_name": query_value(query, "serviceName", "service-name"),
        "download_settings": download_settings,
    }


def parse_trojan_uri(uri: str) -> dict:
    parsed = urllib.parse.urlsplit(normalize_uri(uri))
    query = urllib.parse.parse_qs(parsed.query, keep_blank_values=True)
    password = urllib.parse.unquote(parsed.username or "")
    if not parsed.hostname or parsed.port is None or not password:
        fail("invalid trojan uri")
    return {
        "scheme": "trojan",
        "server": parsed.hostname,
        "port": parsed.port,
        "password": password,
        "network": (query_value(query, "type") or "tcp").lower(),
        "security": (query_value(query, "security") or "tls").lower(),
        "alpn": split_csv(query_value(query, "alpn")),
        "servername": query_value(query, "sni", "servername", "serverName"),
        "fingerprint": query_value(query, "fp", "fingerprint", "client-fingerprint"),
        "skip_cert_verify": is_truthy(query_value(query, "insecure", "allowInsecure", "skip-cert-verify"))
        if has_query_key(query, "insecure", "allowInsecure", "skip-cert-verify")
        else None,
        "path": query_value(query, "path"),
        "host": query_value(query, "host"),
        "service_name": query_value(query, "serviceName", "service-name"),
    }


def decode_ss_authority(parsed: urllib.parse.SplitResult) -> tuple[str, str, str, int]:
    remainder = normalize_uri(parsed.geturl())[len("ss://") :]
    if "#" in remainder:
        remainder = remainder.split("#", 1)[0]
    if "?" in remainder:
        remainder = remainder.split("?", 1)[0]
    if "@" not in remainder:
        decoded = b64decode_padded(remainder).decode("utf-8", errors="ignore")
    else:
        try:
            prefix, suffix = remainder.rsplit("@", 1)
            decoded_prefix = b64decode_padded(prefix).decode("utf-8", errors="ignore")
            decoded = f"{decoded_prefix}@{suffix}" if "@" not in decoded_prefix else remainder
        except Exception:
            decoded = remainder
    if "@" not in decoded:
        fail("invalid ss uri")
    creds, host_port = decoded.rsplit("@", 1)
    if ":" not in creds or ":" not in host_port:
        fail("invalid ss uri")
    method, password = creds.split(":", 1)
    server, port_text = host_port.rsplit(":", 1)
    port = int_value(port_text)
    if not method or not password or not server or port is None:
        fail("invalid ss uri")
    return method, urllib.parse.unquote(password), server, port


def parse_ss_plugin(value: str) -> tuple[str, dict]:
    plugin, *raw_opts = value.split(";")
    result: dict[str, object] = {}
    for opt in raw_opts:
        if "=" not in opt:
            continue
        key, val = opt.split("=", 1)
        key = key.strip()
        val = val.strip()
        if key in {"tls", "mux"}:
            result[key] = is_truthy(val)
        else:
            result[key] = val
    return plugin.strip(), result


def parse_ss_uri(uri: str) -> dict:
    parsed = urllib.parse.urlsplit(normalize_uri(uri))
    query = urllib.parse.parse_qs(parsed.query, keep_blank_values=True)
    method, password, server, port = decode_ss_authority(parsed)
    plugin_name = ""
    plugin_opts = {}
    plugin = query_value(query, "plugin")
    if plugin:
        plugin_name, plugin_opts = parse_ss_plugin(plugin)
    return {
        "scheme": "ss",
        "server": server,
        "port": port,
        "cipher": method,
        "password": password,
        "plugin": plugin_name,
        "plugin_opts": plugin_opts,
    }


def parse_vmess_uri(uri: str) -> dict:
    data = parse_json_from_uri_payload(uri)
    server = string_value(data, "add", "server", "address")
    port = int_value(object_value(data, "port"))
    uuid_value = string_value(data, "id", "uuid")
    if not server or port is None or not uuid_value:
        fail("invalid vmess uri")
    tls_flag = string_value(data, "tls", "security").lower()
    network = string_value(data, "net", "network").lower() or "tcp"
    host = string_value(data, "host")
    path = string_value(data, "path")
    service_name = string_value(data, "serviceName", "service-name")
    return {
        "scheme": "vmess",
        "server": server,
        "port": port,
        "uuid": uuid_value,
        "network": network,
        "alter_id": int_value(object_value(data, "aid", "alterId")) or 0,
        "cipher": string_value(data, "scy", "cipher") or "auto",
        "tls": tls_flag in {"tls", "1", "true"},
        "servername": string_value(data, "sni", "servername"),
        "fingerprint": string_value(data, "fp", "fingerprint"),
        "alpn": split_csv(string_value(data, "alpn")),
        "skip_cert_verify": is_truthy(string_value(data, "allowInsecure", "insecure"))
        if string_value(data, "allowInsecure", "insecure")
        else None,
        "host": host,
        "path": path,
        "service_name": service_name or path.lstrip("/"),
        "header_type": string_value(data, "type"),
    }


def parse_uri_info(uri: str) -> dict:
    raw = normalize_uri(uri)
    if "://" not in raw:
        fail("invalid uri")
    scheme = raw.split("://", 1)[0].lower()
    if scheme == "vless":
        return parse_vless_uri(raw)
    if scheme == "trojan":
        return parse_trojan_uri(raw)
    if scheme == "ss":
        return parse_ss_uri(raw)
    if scheme == "vmess":
        return parse_vmess_uri(raw)
    fail(f"unsupported scheme: {scheme}")


def vmess_base_key(uri: str) -> str:
    data = parse_json_from_uri_payload(uri)
    data["ps"] = ""
    encoded = base64.b64encode(json.dumps(data, ensure_ascii=False, separators=(",", ":")).encode("utf-8")).decode("ascii")
    return f"vmess://{encoded}"


def uri_base_key(uri: str) -> str:
    raw = normalize_uri(uri)
    if raw.startswith("vmess://"):
        return vmess_base_key(raw)
    parsed = urllib.parse.urlsplit(raw)
    return urllib.parse.urlunsplit(parsed._replace(fragment=""))


def guess_name(uri: str) -> str:
    raw = normalize_uri(uri)
    if raw.startswith("vmess://"):
        try:
            data = parse_json_from_uri_payload(raw)
            name = string_value(data, "ps")
            if name:
                return name
            host = string_value(data, "add", "server", "address") or "node"
            network = string_value(data, "net", "network").lower() or "tcp"
            return f"{network}-{host}"
        except (Exception, SystemExit):
            return "vmess-node"
    parsed = urllib.parse.urlsplit(raw)
    if parsed.fragment:
        return urllib.parse.unquote(parsed.fragment)
    host = parsed.hostname or "node"
    if parsed.scheme == "ss":
        return f"ss-{host}"
    query = urllib.parse.parse_qs(parsed.query, keep_blank_values=True)
    network = (query.get("type", ["tcp"])[0] or "tcp").lower()
    return f"{network}-{host}"


def safe_split_port(parsed: urllib.parse.SplitResult) -> str:
    try:
        port = parsed.port
    except ValueError:
        return ""
    return str(port or "")


def unsupported_uri_info(raw: str, scheme: str, reason: str) -> dict:
    parsed = urllib.parse.urlsplit(raw)
    query = urllib.parse.parse_qs(parsed.query, keep_blank_values=True)
    return {
        "name": guess_name(raw),
        "server": parsed.hostname or "",
        "port": safe_split_port(parsed),
        "network": (query.get("type", ["tcp"])[0] or "tcp").lower(),
        "security": (query.get("security", [""])[0] or scheme).lower(),
        "scheme": scheme,
        "supported": "0",
        "reason": reason,
    }


def uri_info(uri: str) -> dict:
    raw = normalize_uri(uri)
    scheme = raw.split("://", 1)[0].lower() if "://" in raw else ""
    try:
        info = parse_uri_info(raw)
        return {
            "name": guess_name(raw),
            "server": str(info.get("server", "")),
            "port": str(info.get("port", "") or ""),
            "network": str(info.get("network", "tcp") or "tcp"),
            "security": str(info.get("security", scheme) or scheme),
            "scheme": scheme,
            "supported": "1",
            "reason": "",
        }
    except SystemExit as exc:
        return unsupported_uri_info(raw, scheme, str(exc) or f"unsupported scheme: {scheme}")
    except Exception as exc:
        return unsupported_uri_info(raw, scheme, str(exc) or f"invalid {scheme or 'unknown'} uri")


def reality_opts_from_mapping(mapping: dict | None) -> dict:
    result: dict[str, object] = {}
    public_key = string_value(mapping, "publicKey", "public-key")
    short_id = string_value(mapping, "shortId", "short-id")
    spider_x = string_value(mapping, "spiderX", "spider-x")
    if public_key:
        result["public-key"] = public_key
    if short_id:
        result["short-id"] = short_id
    if spider_x:
        result["spider-x"] = spider_x
    return result


def reality_opts_from_query(query: dict[str, list[str]]) -> dict:
    result: dict[str, object] = {}
    public_key = query_value(query, "pbk", "publicKey", "public-key")
    short_id = query_value(query, "sid", "shortId", "short-id")
    spider_x = query_value(query, "spx", "spiderX", "spider-x")
    if public_key:
        result["public-key"] = public_key
    if short_id:
        result["short-id"] = short_id
    if spider_x:
        result["spider-x"] = spider_x
    return result


def xhttp_download_settings_from_mapping(mapping: dict | None) -> dict:
    if not isinstance(mapping, dict):
        return {}
    result: dict[str, object] = {}
    xhttp_settings = object_value(mapping, "xhttpSettings", "xhttp-settings")
    path = string_value(xhttp_settings, "path")
    host = string_value(xhttp_settings, "host")
    mode = string_value(xhttp_settings, "mode")
    server = string_value(mapping, "address", "server")
    port = int_value(object_value(mapping, "port"))
    security = string_value(mapping, "security").lower()
    if path:
        result["path"] = path
    if host:
        result["host"] = host
    if mode:
        result["mode"] = mode
    if server:
        result["server"] = server
    if port is not None:
        result["port"] = port
    if security in {"tls", "reality"}:
        result["tls"] = True
    if security == "reality":
        reality_settings = object_value(mapping, "realitySettings", "reality-opts")
        server_name = string_value(reality_settings, "serverName", "servername", "server-name", "sni")
        fingerprint = string_value(reality_settings, "fingerprint", "fp", "client-fingerprint")
        reality_opts = reality_opts_from_mapping(reality_settings)
        if server_name:
            result["servername"] = server_name
        if fingerprint:
            result["client-fingerprint"] = fingerprint
        if reality_opts:
            result["reality-opts"] = reality_opts
    else:
        server_name = string_value(mapping, "serverName", "servername", "server-name", "sni")
        fingerprint = string_value(mapping, "fingerprint", "fp", "client-fingerprint")
        if server_name:
            result["servername"] = server_name
        if fingerprint:
            result["client-fingerprint"] = fingerprint
    return result


def apply_common_tls_fields(item: dict[str, object], info: dict) -> None:
    if info.get("alpn"):
        item["alpn"] = info["alpn"]
    if info.get("servername"):
        item["servername"] = info["servername"]
    if info.get("fingerprint"):
        item["client-fingerprint"] = info["fingerprint"]
    if info.get("skip_cert_verify") is not None:
        item["skip-cert-verify"] = info["skip_cert_verify"]


def apply_network_opts(item: dict[str, object], info: dict) -> None:
    network = str(info.get("network", "tcp") or "tcp").lower()
    item["network"] = network
    path = str(info.get("path", "") or "")
    host = str(info.get("host", "") or "")
    service_name = str(info.get("service_name", "") or "")
    header_type = str(info.get("header_type", "") or "")
    if network == "ws":
        opts: dict[str, object] = {}
        if path:
            opts["path"] = path
        if host:
            opts["headers"] = {"Host": host}
        if opts:
            item["ws-opts"] = opts
    elif network == "grpc":
        if service_name:
            item["grpc-opts"] = {"grpc-service-name": service_name}
    elif network == "httpupgrade":
        opts = {}
        if path:
            opts["path"] = path
        if host:
            opts["host"] = host
        if opts:
            item["http-upgrade-opts"] = opts
    elif network in {"http", "h2"}:
        opts = {}
        if host:
            opts["host"] = [host]
        if path:
            opts["path"] = path
        if opts:
            item["h2-opts"] = opts
    elif network == "tcp" and header_type:
        item["header"] = {"type": header_type}


def provider_item_from_node(node: dict) -> dict:
    info = parse_uri_info(node["uri"])
    scheme = info["scheme"]
    name = node["name"]
    if scheme == "vless":
        item: dict[str, object] = {
            "name": name,
            "type": "vless",
            "server": info["server"],
            "port": info["port"],
            "uuid": info["uuid"],
            "udp": True,
        }
        if info.get("flow"):
            item["flow"] = info["flow"]
        if info.get("packet_encoding"):
            item["packet-encoding"] = info["packet_encoding"]
        if info.get("encryption"):
            item["encryption"] = info["encryption"]
        if info.get("security") in {"tls", "reality"}:
            item["tls"] = True
        apply_common_tls_fields(item, info)
        if info.get("security") == "reality" and info.get("reality_opts"):
            item["reality-opts"] = info["reality_opts"]
        apply_network_opts(item, info)
        if item.get("network") == "xhttp":
            xhttp_opts: dict[str, object] = {}
            if info.get("path"):
                xhttp_opts["path"] = info["path"]
            if info.get("host"):
                xhttp_opts["host"] = info["host"]
            if info.get("mode"):
                xhttp_opts["mode"] = info["mode"]
            rendered_download_settings = xhttp_download_settings_from_mapping(info.get("download_settings"))
            if rendered_download_settings:
                xhttp_opts["download-settings"] = rendered_download_settings
            if xhttp_opts:
                item["xhttp-opts"] = xhttp_opts
        return item
    if scheme == "trojan":
        item = {
            "name": name,
            "type": "trojan",
            "server": info["server"],
            "port": info["port"],
            "password": info["password"],
            "udp": True,
            "tls": info.get("security", "tls") in {"tls", "reality"},
        }
        apply_common_tls_fields(item, info)
        apply_network_opts(item, info)
        return item
    if scheme == "ss":
        item = {
            "name": name,
            "type": "ss",
            "server": info["server"],
            "port": info["port"],
            "cipher": info["cipher"],
            "password": info["password"],
            "udp": True,
        }
        if info.get("plugin"):
            item["plugin"] = info["plugin"]
        if info.get("plugin_opts"):
            item["plugin-opts"] = info["plugin_opts"]
        return item
    if scheme == "vmess":
        item = {
            "name": name,
            "type": "vmess",
            "server": info["server"],
            "port": info["port"],
            "uuid": info["uuid"],
            "alterId": info["alter_id"],
            "cipher": info["cipher"],
            "udp": True,
        }
        if info.get("tls"):
            item["tls"] = True
        apply_common_tls_fields(item, info)
        apply_network_opts(item, info)
        return item
    fail(f"unsupported scheme: {scheme}")


def render_provider(path: Path, nodes: list[dict]) -> None:
    ensure_parent(path)
    enabled = [provider_item_from_node(node) for node in nodes if node.get("enabled")]
    if not enabled:
        path.write_text("proxies: []\n", encoding="utf-8")
        return
    lines = ["proxies:"]
    append_yaml_lines(lines, enabled, 2)
    path.write_text("\n".join(lines) + "\n", encoding="utf-8")


def render_rules(path: Path, rules: list[dict]) -> None:
    ensure_parent(path)
    lines = [f"{RULE_KIND_MAP[rule['kind']]},{rule['pattern']},{rule['target']}" for rule in rules]
    path.write_text(("\n".join(lines) + "\n") if lines else "", encoding="utf-8")


def scan_uri_rows(text: str) -> list[dict]:
    rows = []
    for line in decode_subscription_lines(text):
        uri = normalize_uri(line)
        if "://" not in uri:
            continue
        info = uri_info(uri)
        rows.append(
            {
                "uri": uri,
                "name": info["name"],
                "server": info["server"],
                "port": info["port"],
                "network": info["network"],
                "security": info["security"],
                "supported": info["supported"],
                "scheme": info["scheme"],
                "reason": info["reason"],
            }
        )
    return rows


def make_node(name: str, uri: str, enabled: bool, source_kind: str = "manual", source_id: str = "") -> dict:
    node = {
        "id": str(uuid.uuid4()),
        "name": name,
        "enabled": enabled,
        "uri": normalize_uri(uri),
        "imported_at": now_iso(),
        "source": {"kind": source_kind},
    }
    if source_id:
        node["source"]["id"] = source_id
    return node


def remove_nodes_by_source(nodes: list[dict], source_kind: str, source_id: str) -> list[dict]:
    remaining = []
    for node in nodes:
        source = node.get("source") or {}
        if source.get("kind") == source_kind and source.get("id") == source_id:
            continue
        remaining.append(node)
    return remaining


def find_subscription(state: dict, subscription_id: str) -> dict:
    for item in state["subscriptions"]:
        if item["id"] == subscription_id:
            return item
    fail(f"subscription not found: {subscription_id}")


def cmd_scan_uris(args: argparse.Namespace) -> int:
    text = Path(args.input_file).read_text(encoding="utf-8", errors="ignore")
    for row in scan_uri_rows(text):
        print(
            "\t".join(
                [
                    row["uri"],
                    row["name"],
                    row["server"],
                    row["port"],
                    row["network"],
                    row["security"],
                    row["supported"],
                    row["scheme"],
                    row["reason"],
                ]
            )
        )
    return 0


def cmd_ensure_nodes_state(args: argparse.Namespace) -> int:
    ensure_nodes_state(Path(args.state_file), Path(args.legacy_file) if args.legacy_file else None)
    return 0


def cmd_ensure_rules_state(args: argparse.Namespace) -> int:
    ensure_rules_state(Path(args.state_file), Path(args.legacy_file) if args.legacy_file else None)
    return 0


def cmd_ensure_subscriptions_state(args: argparse.Namespace) -> int:
    ensure_subscriptions_state(Path(args.state_file))
    return 0


def cmd_list_nodes(args: argparse.Namespace) -> int:
    state = ensure_nodes_state(Path(args.state_file))
    for idx, node in enumerate(state["nodes"], start=1):
        info = uri_info(node["uri"])
        source = node.get("source") or {}
        source_label = source.get("kind", "manual")
        if source.get("id"):
            source_label = f"{source_label}:{source['id']}"
        print(
            "\t".join(
                [
                    str(idx),
                    "1" if node.get("enabled") else "0",
                    node["name"],
                    info["server"],
                    info["port"],
                    info["network"],
                    info["security"],
                    node["id"],
                    source_label,
                ]
            )
        )
    return 0


def cmd_append_node(args: argparse.Namespace) -> int:
    path = Path(args.state_file)
    state = ensure_nodes_state(path)
    uri = normalize_uri(args.uri)
    parse_uri_info(uri)
    name = args.name or guess_name(uri)
    enabled = args.enabled == "1"
    source_kind = args.source_kind or "manual"
    source_id = args.source_id or ""
    base_key = uri_base_key(uri)
    for node in state["nodes"]:
        if uri_base_key(node["uri"]) == base_key:
            ensure_unique_name(state["nodes"], name, ignore_id=node["id"])
            node["name"] = name
            node["enabled"] = enabled
            node["uri"] = uri
            node["source"] = {"kind": source_kind}
            if source_id:
                node["source"]["id"] = source_id
            save_json(path, state)
            print("updated")
            return 0
    ensure_unique_name(state["nodes"], name)
    state["nodes"].append(make_node(name, uri, enabled, source_kind=source_kind, source_id=source_id))
    save_json(path, state)
    print("added")
    return 0


def cmd_sync_subscription_nodes(args: argparse.Namespace) -> int:
    path = Path(args.state_file)
    state = ensure_nodes_state(path)
    text = Path(args.input_file).read_text(encoding="utf-8", errors="ignore")
    state["nodes"] = remove_nodes_by_source(state["nodes"], "subscription", args.subscription_id)
    imported = 0
    for row in scan_uri_rows(text):
        if row["supported"] != "1":
            continue
        preferred = row["name"] or guess_name(row["uri"])
        name = resolve_unique_name(state["nodes"], preferred)
        state["nodes"].append(make_node(name, row["uri"], True, source_kind="subscription", source_id=args.subscription_id))
        imported += 1
    save_json(path, state)
    print(imported)
    return 0


def cmd_purge_subscription_nodes(args: argparse.Namespace) -> int:
    path = Path(args.state_file)
    state = ensure_nodes_state(path)
    before = len(state["nodes"])
    state["nodes"] = remove_nodes_by_source(state["nodes"], "subscription", args.subscription_id)
    save_json(path, state)
    print(before - len(state["nodes"]))
    return 0


def cmd_rename_node(args: argparse.Namespace) -> int:
    path = Path(args.state_file)
    state = ensure_nodes_state(path)
    index = int(args.index) - 1
    if index < 0 or index >= len(state["nodes"]):
        fail("node index out of range")
    node = state["nodes"][index]
    ensure_unique_name(state["nodes"], args.new_name, ignore_id=node["id"])
    old_name = node["name"]
    node["name"] = args.new_name
    save_json(path, state)
    for rules_state_file in args.rule_state_files or []:
        rules_path = Path(rules_state_file)
        rules_state = ensure_rules_state(rules_path)
        for rule in rules_state["rules"]:
            if rule["target"] == old_name:
                rule["target"] = args.new_name
        save_json(rules_path, rules_state)
    return 0


def cmd_set_node_enabled(args: argparse.Namespace) -> int:
    path = Path(args.state_file)
    state = ensure_nodes_state(path)
    index = int(args.index) - 1
    if index < 0 or index >= len(state["nodes"]):
        fail("node index out of range")
    state["nodes"][index]["enabled"] = args.enabled == "1"
    save_json(path, state)
    return 0


def cmd_enabled_count(args: argparse.Namespace) -> int:
    state = ensure_nodes_state(Path(args.state_file))
    print(sum(1 for node in state["nodes"] if node.get("enabled")))
    return 0


def cmd_enabled_names(args: argparse.Namespace) -> int:
    state = ensure_nodes_state(Path(args.state_file))
    for name in iter_enabled_names(state["nodes"]):
        print(name)
    return 0


def cmd_all_names(args: argparse.Namespace) -> int:
    state = ensure_nodes_state(Path(args.state_file))
    for node in state["nodes"]:
        print(node["name"])
    return 0


def cmd_add_rule(args: argparse.Namespace) -> int:
    path = Path(args.state_file)
    state = ensure_rules_state(path)
    kind = normalize_rule_kind(args.kind)
    rule = {"id": str(uuid.uuid4()), "kind": kind, "pattern": args.pattern, "target": args.target}
    if not any(r["kind"] == rule["kind"] and r["pattern"] == rule["pattern"] and r["target"] == rule["target"] for r in state["rules"]):
        state["rules"].append(rule)
        save_json(path, state)
    return 0


def cmd_list_rules(args: argparse.Namespace) -> int:
    state = ensure_rules_state(Path(args.state_file))
    for idx, rule in enumerate(state["rules"], start=1):
        print(f"{idx}\t{RULE_KIND_MAP[rule['kind']]},{rule['pattern']},{rule['target']}")
    return 0


def cmd_remove_rule(args: argparse.Namespace) -> int:
    path = Path(args.state_file)
    state = ensure_rules_state(path)
    index = int(args.index) - 1
    if index < 0 or index >= len(state["rules"]):
        fail("rule index out of range")
    del state["rules"][index]
    save_json(path, state)
    return 0


def cmd_render_provider(args: argparse.Namespace) -> int:
    state = ensure_nodes_state(Path(args.state_file))
    render_provider(Path(args.output_file), state["nodes"])
    return 0


def cmd_render_rules(args: argparse.Namespace) -> int:
    state = ensure_rules_state(Path(args.state_file))
    render_rules(Path(args.output_file), state["rules"])
    return 0


def cmd_validate_rule_targets(args: argparse.Namespace) -> int:
    rules_state = ensure_rules_state(Path(args.rules_state))
    nodes_state = ensure_nodes_state(Path(args.nodes_state))
    enabled_names = set(iter_enabled_names(nodes_state["nodes"]))
    allowed = {"DIRECT", "PROXY", "REJECT"} | enabled_names
    if enabled_names:
        allowed.add("AUTO")
    invalid = []
    for idx, rule in enumerate(rules_state["rules"], start=1):
        if rule["target"] not in allowed:
            invalid.append(f"{idx}:{rule['target']}")
    if invalid:
        print("\n".join(invalid))
        return 1
    return 0


def cmd_list_subscriptions(args: argparse.Namespace) -> int:
    state = ensure_subscriptions_state(Path(args.state_file))
    for idx, item in enumerate(state["subscriptions"], start=1):
        print(
            "\t".join(
                [
                    str(idx),
                    item["id"],
                    item["name"],
                    item["url"],
                    "1" if item.get("enabled", True) else "0",
                    item.get("last_success_at", ""),
                    str(item.get("last_imported_count", 0)),
                    item.get("last_error", ""),
                ]
            )
        )
    return 0


def cmd_append_subscription(args: argparse.Namespace) -> int:
    path = Path(args.state_file)
    state = ensure_subscriptions_state(path)
    name = args.name.strip() or "subscription"
    for item in state["subscriptions"]:
        if item["url"] == args.url:
            item["name"] = name
            item["enabled"] = args.enabled == "1"
            save_json(path, state)
            print(item["id"])
            return 0
    item = {
        "id": str(uuid.uuid4()),
        "name": name,
        "url": args.url,
        "enabled": args.enabled == "1",
        "created_at": now_iso(),
        "last_updated_at": "",
        "last_success_at": "",
        "last_imported_count": 0,
        "last_error": "",
    }
    state["subscriptions"].append(item)
    save_json(path, state)
    print(item["id"])
    return 0


def cmd_set_subscription_enabled(args: argparse.Namespace) -> int:
    path = Path(args.state_file)
    state = ensure_subscriptions_state(path)
    item = find_subscription(state, args.subscription_id)
    item["enabled"] = args.enabled == "1"
    save_json(path, state)
    return 0


def cmd_remove_subscription(args: argparse.Namespace) -> int:
    path = Path(args.state_file)
    state = ensure_subscriptions_state(path)
    before = len(state["subscriptions"])
    state["subscriptions"] = [item for item in state["subscriptions"] if item["id"] != args.subscription_id]
    if len(state["subscriptions"]) == before:
        fail(f"subscription not found: {args.subscription_id}")
    save_json(path, state)
    return 0


def cmd_mark_subscription_success(args: argparse.Namespace) -> int:
    path = Path(args.state_file)
    state = ensure_subscriptions_state(path)
    item = find_subscription(state, args.subscription_id)
    timestamp = now_iso()
    item["last_updated_at"] = timestamp
    item["last_success_at"] = timestamp
    item["last_imported_count"] = int(args.imported_count)
    item["last_error"] = ""
    save_json(path, state)
    return 0


def cmd_mark_subscription_error(args: argparse.Namespace) -> int:
    path = Path(args.state_file)
    state = ensure_subscriptions_state(path)
    item = find_subscription(state, args.subscription_id)
    item["last_updated_at"] = now_iso()
    item["last_error"] = args.message
    save_json(path, state)
    return 0


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser()
    sub = parser.add_subparsers(dest="command", required=True)

    scan = sub.add_parser("scan-uris")
    scan.add_argument("input_file")
    scan.set_defaults(func=cmd_scan_uris)

    ensure_nodes = sub.add_parser("ensure-nodes-state")
    ensure_nodes.add_argument("state_file")
    ensure_nodes.add_argument("legacy_file", nargs="?")
    ensure_nodes.set_defaults(func=cmd_ensure_nodes_state)

    ensure_rules = sub.add_parser("ensure-rules-state")
    ensure_rules.add_argument("state_file")
    ensure_rules.add_argument("legacy_file", nargs="?")
    ensure_rules.set_defaults(func=cmd_ensure_rules_state)

    ensure_subscriptions = sub.add_parser("ensure-subscriptions-state")
    ensure_subscriptions.add_argument("state_file")
    ensure_subscriptions.set_defaults(func=cmd_ensure_subscriptions_state)

    list_nodes = sub.add_parser("list-nodes")
    list_nodes.add_argument("state_file")
    list_nodes.set_defaults(func=cmd_list_nodes)

    append_node = sub.add_parser("append-node")
    append_node.add_argument("state_file")
    append_node.add_argument("uri")
    append_node.add_argument("name")
    append_node.add_argument("enabled")
    append_node.add_argument("source_kind", nargs="?")
    append_node.add_argument("source_id", nargs="?")
    append_node.set_defaults(func=cmd_append_node)

    sync_nodes = sub.add_parser("sync-subscription-nodes")
    sync_nodes.add_argument("state_file")
    sync_nodes.add_argument("subscription_id")
    sync_nodes.add_argument("input_file")
    sync_nodes.set_defaults(func=cmd_sync_subscription_nodes)

    purge_nodes = sub.add_parser("purge-subscription-nodes")
    purge_nodes.add_argument("state_file")
    purge_nodes.add_argument("subscription_id")
    purge_nodes.set_defaults(func=cmd_purge_subscription_nodes)

    rename_node = sub.add_parser("rename-node")
    rename_node.add_argument("state_file")
    rename_node.add_argument("index")
    rename_node.add_argument("new_name")
    rename_node.add_argument("rule_state_files", nargs="*")
    rename_node.set_defaults(func=cmd_rename_node)

    set_enabled = sub.add_parser("set-node-enabled")
    set_enabled.add_argument("state_file")
    set_enabled.add_argument("index")
    set_enabled.add_argument("enabled")
    set_enabled.set_defaults(func=cmd_set_node_enabled)

    enabled_count = sub.add_parser("enabled-count")
    enabled_count.add_argument("state_file")
    enabled_count.set_defaults(func=cmd_enabled_count)

    enabled_names = sub.add_parser("enabled-names")
    enabled_names.add_argument("state_file")
    enabled_names.set_defaults(func=cmd_enabled_names)

    all_names = sub.add_parser("all-names")
    all_names.add_argument("state_file")
    all_names.set_defaults(func=cmd_all_names)

    add_rule = sub.add_parser("add-rule")
    add_rule.add_argument("state_file")
    add_rule.add_argument("kind")
    add_rule.add_argument("pattern")
    add_rule.add_argument("target")
    add_rule.set_defaults(func=cmd_add_rule)

    list_rules = sub.add_parser("list-rules")
    list_rules.add_argument("state_file")
    list_rules.set_defaults(func=cmd_list_rules)

    remove_rule = sub.add_parser("remove-rule")
    remove_rule.add_argument("state_file")
    remove_rule.add_argument("index")
    remove_rule.set_defaults(func=cmd_remove_rule)

    render_provider_cmd = sub.add_parser("render-provider")
    render_provider_cmd.add_argument("state_file")
    render_provider_cmd.add_argument("output_file")
    render_provider_cmd.set_defaults(func=cmd_render_provider)

    render_rules_cmd = sub.add_parser("render-rules")
    render_rules_cmd.add_argument("state_file")
    render_rules_cmd.add_argument("output_file")
    render_rules_cmd.set_defaults(func=cmd_render_rules)

    validate_targets = sub.add_parser("validate-rule-targets")
    validate_targets.add_argument("rules_state")
    validate_targets.add_argument("nodes_state")
    validate_targets.set_defaults(func=cmd_validate_rule_targets)

    list_subscriptions = sub.add_parser("list-subscriptions")
    list_subscriptions.add_argument("state_file")
    list_subscriptions.set_defaults(func=cmd_list_subscriptions)

    append_subscription = sub.add_parser("append-subscription")
    append_subscription.add_argument("state_file")
    append_subscription.add_argument("name")
    append_subscription.add_argument("url")
    append_subscription.add_argument("enabled")
    append_subscription.set_defaults(func=cmd_append_subscription)

    set_subscription = sub.add_parser("set-subscription-enabled")
    set_subscription.add_argument("state_file")
    set_subscription.add_argument("subscription_id")
    set_subscription.add_argument("enabled")
    set_subscription.set_defaults(func=cmd_set_subscription_enabled)

    remove_subscription = sub.add_parser("remove-subscription")
    remove_subscription.add_argument("state_file")
    remove_subscription.add_argument("subscription_id")
    remove_subscription.set_defaults(func=cmd_remove_subscription)

    mark_success = sub.add_parser("mark-subscription-success")
    mark_success.add_argument("state_file")
    mark_success.add_argument("subscription_id")
    mark_success.add_argument("imported_count")
    mark_success.set_defaults(func=cmd_mark_subscription_success)

    mark_error = sub.add_parser("mark-subscription-error")
    mark_error.add_argument("state_file")
    mark_error.add_argument("subscription_id")
    mark_error.add_argument("message")
    mark_error.set_defaults(func=cmd_mark_subscription_error)

    return parser


def main() -> int:
    parser = build_parser()
    args = parser.parse_args()
    return args.func(args)


if __name__ == "__main__":
    raise SystemExit(main())
