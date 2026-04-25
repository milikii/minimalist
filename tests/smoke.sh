#!/usr/bin/env bash

set -euo pipefail

ROOT="$(cd "$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")/.." && pwd)"
MANAGER="${ROOT}/mihomo"
STATECTL="${ROOT}/scripts/statectl.py"
RULEPRESETCTL="${ROOT}/scripts/rulepreset.py"

cleanup() {
  [[ -n "${TMPDIR_CASE:-}" && -d "${TMPDIR_CASE:-}" ]] && rm -rf "$TMPDIR_CASE"
}
trap cleanup EXIT

assert_contains() {
  local haystack="$1"
  local needle="$2"
  if [[ "$haystack" != *"$needle"* ]]; then
    printf 'assert_contains failed: expected [%s] in output\n' "$needle" >&2
    exit 1
  fi
}

setup_case() {
  TMPDIR_CASE="$(mktemp -d)"
  mkdir -p "${TMPDIR_CASE}/state" "${TMPDIR_CASE}/ruleset" "${TMPDIR_CASE}/proxy_providers" "${TMPDIR_CASE}/ui"
  cp -a "${ROOT}/rules-repo" "${TMPDIR_CASE}/rules-repo"
  cat > "${TMPDIR_CASE}/router.env" <<'EOF'
TEMPLATE_NAME="nas-single-lan-v4"
ENABLE_IPV6="0"
LAN_INTERFACES="bridge1"
LAN_CIDRS="192.168.2.0/24"
PROXY_INGRESS_INTERFACES="bridge1"
DNS_HIJACK_ENABLED="1"
DNS_HIJACK_INTERFACES="bridge1"
PROXY_HOST_OUTPUT="0"
BYPASS_CONTAINER_NAMES=""
BYPASS_SRC_CIDRS=""
BYPASS_DST_CIDRS=""
BYPASS_UIDS=""
MIXED_PORT="7890"
TPROXY_PORT="7893"
DNS_PORT="1053"
CONTROLLER_PORT="19090"
CONTROLLER_BIND_ADDRESS="127.0.0.1"
ROUTE_MARK="0x2333"
ROUTE_MASK="0xffffffff"
ROUTE_TABLE="233"
ROUTE_PRIORITY="100"
EOF
}

env_prefix() {
  printf 'APP_ROOT=%q INSTALL_ROOT=%q RULE_REPO_ROOT=%q MIHOMO_DIR=%q SETTINGS_ENV=%q ROUTER_ENV=%q CONFIG_FILE=%q RULES_DIR=%q PROVIDER_DIR=%q UI_DIR=%q STATE_DIR=%q NODES_STATE_FILE=%q RULES_STATE_FILE=%q ACL_STATE_FILE=%q SUBSCRIPTIONS_STATE_FILE=%q PROVIDER_FILE=%q RENDERED_RULES_FILE=%q ACL_RENDERED_RULES_FILE=%q MIHOMO_USER=%q MANAGER_BIN=%q MIHOMO_BIN=%q' \
    "$ROOT" \
    "${TMPDIR_CASE}/install-root" \
    "${TMPDIR_CASE}/rules-repo" \
    "$TMPDIR_CASE" \
    "$TMPDIR_CASE/settings.env" \
    "$TMPDIR_CASE/router.env" \
    "$TMPDIR_CASE/config.yaml" \
    "$TMPDIR_CASE/ruleset" \
    "$TMPDIR_CASE/proxy_providers" \
    "$TMPDIR_CASE/ui" \
    "$TMPDIR_CASE/state" \
    "$TMPDIR_CASE/state/nodes.json" \
    "$TMPDIR_CASE/state/rules.json" \
    "$TMPDIR_CASE/state/acl.json" \
    "$TMPDIR_CASE/state/subscriptions.json" \
    "$TMPDIR_CASE/proxy_providers/manual.txt" \
    "$TMPDIR_CASE/ruleset/custom.rules" \
    "$TMPDIR_CASE/ruleset/acl.rules" \
    root \
    "$TMPDIR_CASE/mihomo" \
    /bin/true
}

run_manager() {
  local cmd
  cmd="$(env_prefix)"
  # shellcheck disable=SC2086
  eval "$cmd" "$MANAGER" "$@"
}

test_syntax() {
  bash -n "${ROOT}/mihomo" "${ROOT}/lib/common.sh" "${ROOT}/lib/render.sh"
  python3 -m py_compile "${STATECTL}"
  python3 -m py_compile "${RULEPRESETCTL}"
}

test_render_empty() {
  setup_case
  run_manager render-config >/dev/null
  grep -q '^RULESET_PRESET="default"$' "${TMPDIR_CASE}/settings.env"
  grep -q 'DOMAIN-SUFFIX,smzdm.com,DIRECT' "${TMPDIR_CASE}/ruleset/builtin.rules"
  grep -q '^proxies: \[\]' "${TMPDIR_CASE}/proxy_providers/manual.txt"
  grep -q '^ipv6: false' "${TMPDIR_CASE}/config.yaml"
  grep -q '^  ipv6: false' "${TMPDIR_CASE}/config.yaml"
  grep -q '^  - 192.168.2.0/24' "${TMPDIR_CASE}/config.yaml"
  grep -q '^  - 127.0.0.0/8' "${TMPDIR_CASE}/config.yaml"
  if grep -q '^lan-disallowed-ips:' "${TMPDIR_CASE}/config.yaml"; then
    echo "lan-disallowed-ips should be absent by default" >&2
    exit 1
  fi
  if grep -q '^authentication:' "${TMPDIR_CASE}/config.yaml"; then
    echo "authentication should be absent by default" >&2
    exit 1
  fi
  if grep -q '^external-ui-name:' "${TMPDIR_CASE}/config.yaml"; then
    echo "external-ui-name should be absent by default" >&2
    exit 1
  fi
  if grep -q '^external-ui-url:' "${TMPDIR_CASE}/config.yaml"; then
    echo "external-ui-url should be absent by default" >&2
    exit 1
  fi
  if grep -q '^external-controller-cors:' "${TMPDIR_CASE}/config.yaml"; then
    echo "external-controller-cors should be absent by default" >&2
    exit 1
  fi
  grep -q 'DOMAIN-SUFFIX,smzdm.com,DIRECT' "${TMPDIR_CASE}/config.yaml"
  [[ -f "${TMPDIR_CASE}/state/acl.json" ]]
  [[ -f "${TMPDIR_CASE}/state/subscriptions.json" ]]
  [[ -f "${TMPDIR_CASE}/ruleset/acl.rules" ]]
}

test_protocol_renderers() {
  setup_case
  run_manager render-config >/dev/null
  python3 "${STATECTL}" append-node "${TMPDIR_CASE}/state/nodes.json" 'vless://uuid@example.com:443?encryption=none&security=reality&sni=www.microsoft.com&fp=chrome&pbk=PUBLIC_KEY&sid=abcd&type=tcp#vless-node' vless-node 1 >/dev/null
  python3 "${STATECTL}" append-node "${TMPDIR_CASE}/state/nodes.json" 'trojan://password@example.org:443?security=tls&sni=www.apple.com&type=ws&host=www.apple.com&path=%2Fws#trojan-node' trojan-node 1 >/dev/null
  python3 "${STATECTL}" append-node "${TMPDIR_CASE}/state/nodes.json" 'ss://YWVzLTI1Ni1nY206c2VjcmV0QGV4YW1wbGUubmV0OjQ0Mw==#ss-node' ss-node 1 >/dev/null
  python3 "${STATECTL}" append-node "${TMPDIR_CASE}/state/nodes.json" 'vmess://eyJhZGQiOiJ2bWVzcy5leGFtcGxlLmNvbSIsInBvcnQiOiI0NDMiLCJpZCI6IjEyMzQ1Njc4LTEyMzQtMTIzNC0xMjM0LTEyMzQ1Njc4OTBhYiIsImFpZCI6IjAiLCJuZXQiOiJ3cyIsInR5cGUiOiJub25lIiwiaG9zdCI6Ind3dy5naXRodWIuY29tIiwicGF0aCI6Ii92bWVzcyIsInRscyI6InRscyIsInNuaSI6Ind3dy5naXRodWIuY29tIiwicHMiOiJ2bWVzcy1ub2RlIn0=' vmess-node 1 >/dev/null
  run_manager render-config >/dev/null
  grep -q 'type: "vless"' "${TMPDIR_CASE}/proxy_providers/manual.txt"
  grep -q 'type: "trojan"' "${TMPDIR_CASE}/proxy_providers/manual.txt"
  grep -q 'type: "ss"' "${TMPDIR_CASE}/proxy_providers/manual.txt"
  grep -q 'type: "vmess"' "${TMPDIR_CASE}/proxy_providers/manual.txt"
  grep -q 'name: "vmess-node"' "${TMPDIR_CASE}/proxy_providers/manual.txt"
}

test_acl_rules_are_rendered() {
  setup_case
  run_manager render-config >/dev/null
  python3 "${STATECTL}" append-node "${TMPDIR_CASE}/state/nodes.json" 'vless://uuid@example.com:443?encryption=none&security=reality&sni=www.microsoft.com&fp=chrome&pbk=PUBLIC_KEY&sid=abcd&type=tcp#Proxy-Node' Proxy-Node 1 >/dev/null
  python3 "${STATECTL}" add-rule "${TMPDIR_CASE}/state/acl.json" geosite netflix AUTO >/dev/null
  python3 "${STATECTL}" add-rule "${TMPDIR_CASE}/state/acl.json" port 443 Proxy-Node >/dev/null
  run_manager render-config >/dev/null
  grep -q 'GEOSITE,netflix,AUTO' "${TMPDIR_CASE}/ruleset/acl.rules"
  grep -q 'DST-PORT,443,Proxy-Node' "${TMPDIR_CASE}/ruleset/acl.rules"
  grep -q 'GEOSITE,netflix,AUTO' "${TMPDIR_CASE}/config.yaml"
}

test_auto_without_node_fails() {
  setup_case
  run_manager render-config >/dev/null
  python3 "${STATECTL}" add-rule "${TMPDIR_CASE}/state/acl.json" geosite netflix AUTO >/dev/null
  if run_manager render-config >/tmp/mh-smoke-auto.log 2>&1; then
    echo "AUTO target should have failed without enabled nodes" >&2
    exit 1
  fi
  grep -q 'ACL 规则存在指向不存在或未启用节点的目标' /tmp/mh-smoke-auto.log
}

test_scan_marks_unsupported_scheme() {
  setup_case
  printf '%s\n' 'hy2://password@example.com:443#unsupported' > "${TMPDIR_CASE}/uris.txt"
  output="$(python3 "${STATECTL}" scan-uris "${TMPDIR_CASE}/uris.txt")"
  assert_contains "$output" $'\t0\thy2\t'
}

test_scan_marks_invalid_vmess_payload() {
  setup_case
  printf '%s\n' 'vmess://not-base64!!!' > "${TMPDIR_CASE}/uris.txt"
  output="$(python3 "${STATECTL}" scan-uris "${TMPDIR_CASE}/uris.txt")"
  assert_contains "$output" $'\t0\tvmess\tinvalid vmess payload'
}

test_scan_marks_invalid_ss_payload() {
  setup_case
  printf '%s\n' 'ss://not-base64!!!' > "${TMPDIR_CASE}/uris.txt"
  output="$(python3 "${STATECTL}" scan-uris "${TMPDIR_CASE}/uris.txt")"
  assert_contains "$output" $'\t0\tss\tInvalid base64-encoded string'
}

test_scan_marks_invalid_vless_port() {
  setup_case
  printf '%s\n' 'vless://uuid@example.com:abc?type=tcp#bad-port' > "${TMPDIR_CASE}/uris.txt"
  output="$(python3 "${STATECTL}" scan-uris "${TMPDIR_CASE}/uris.txt")"
  assert_contains "$output" $'\t0\tvless\tPort could not be cast to integer value'
}

test_subscription_state_commands() {
  setup_case
  run_manager add-subscription "test-sub" "https://example.com/sub.txt" 1 >/dev/null
  output="$(run_manager subscriptions)"
  assert_contains "$output" 'test-sub'
  assert_contains "$output" 'https://example.com/sub.txt'
  assert_contains "$output" '缓存'
  assert_contains "$output" '可枚举'
}

test_subscription_state_uses_cache_and_enumeration_subobjects() {
  setup_case
  run_manager add-subscription demo https://example.com/sub.txt 1 >/dev/null
  sub_id="$(python3 "${STATECTL}" list-subscriptions "${TMPDIR_CASE}/state/subscriptions.json" | awk -F'\t' 'NR==1{print $2}')"
  python3 "${STATECTL}" mark-subscription-success "${TMPDIR_CASE}/state/subscriptions.json" "${sub_id}" 2 >/dev/null
  python3 - "${TMPDIR_CASE}/state/subscriptions.json" <<'PY'
import json
import sys

with open(sys.argv[1], "r", encoding="utf-8") as f:
    data = json.load(f)
item = data["subscriptions"][0]
assert "cache" in item, item
assert "enumeration" in item, item
assert item["cache"]["last_success_at"], item
assert item["enumeration"]["last_count"] == 2, item
assert item["enumeration"]["method"] == "uri_scan", item
for legacy in ("last_updated_at", "last_cache_success_at", "last_success_at", "last_enumerated_count", "last_imported_count", "last_error"):
    assert legacy not in item, item
PY
}

test_config_loader_treats_values_as_literals() {
  setup_case
  cat > "${TMPDIR_CASE}/settings.env" <<EOF
PROFILE_TEMPLATE="nas-single-lan-v4"
RULESET_PRESET="\$(touch ${TMPDIR_CASE}/settings-pwned)"
EOF
  cat > "${TMPDIR_CASE}/router.env" <<EOF
TEMPLATE_NAME="nas-single-lan-v4"
ENABLE_IPV6="0"
LAN_INTERFACES="bridge1"
LAN_CIDRS="\$(touch ${TMPDIR_CASE}/router-pwned)"
PROXY_INGRESS_INTERFACES="bridge1"
DNS_HIJACK_ENABLED="1"
DNS_HIJACK_INTERFACES="bridge1"
PROXY_HOST_OUTPUT="0"
BYPASS_CONTAINER_NAMES=""
BYPASS_SRC_CIDRS=""
BYPASS_DST_CIDRS=""
BYPASS_UIDS=""
MIXED_PORT="7890"
TPROXY_PORT="7893"
DNS_PORT="1053"
CONTROLLER_PORT="19090"
CONTROLLER_BIND_ADDRESS="127.0.0.1"
ROUTE_MARK="0x2333"
ROUTE_MASK="0xffffffff"
ROUTE_TABLE="233"
ROUTE_PRIORITY="100"
EOF
  output="$(run_manager status)"
  [[ ! -f "${TMPDIR_CASE}/settings-pwned" ]]
  [[ ! -f "${TMPDIR_CASE}/router-pwned" ]]
  assert_contains "$output" '规则预设: $(touch '
  assert_contains "$output" '局域网网段: $(touch '
}

test_render_config_renders_official_access_fields() {
  setup_case
  cat > "${TMPDIR_CASE}/router.env" <<'EOF'
TEMPLATE_NAME="nas-single-lan-v4"
ENABLE_IPV6="0"
LAN_INTERFACES="bridge1"
LAN_CIDRS="192.168.2.0/24"
LAN_DISALLOWED_CIDRS="192.168.2.10/32 192.168.2.11/32"
PROXY_INGRESS_INTERFACES="bridge1"
DNS_HIJACK_ENABLED="1"
DNS_HIJACK_INTERFACES="bridge1"
PROXY_AUTH_CREDENTIALS="alice:secret bob:pass"
SKIP_AUTH_PREFIXES="127.0.0.1/32 192.168.2.0/24"
PROXY_HOST_OUTPUT="0"
BYPASS_CONTAINER_NAMES=""
BYPASS_SRC_CIDRS=""
BYPASS_DST_CIDRS=""
BYPASS_UIDS=""
MIXED_PORT="7890"
TPROXY_PORT="7893"
DNS_PORT="1053"
CONTROLLER_PORT="19090"
CONTROLLER_BIND_ADDRESS="127.0.0.1"
ROUTE_MARK="0x2333"
ROUTE_MASK="0xffffffff"
ROUTE_TABLE="233"
ROUTE_PRIORITY="100"
EOF
  run_manager render-config >/dev/null
  grep -q '^lan-disallowed-ips:$' "${TMPDIR_CASE}/config.yaml"
  grep -q '^  - 192.168.2.10/32$' "${TMPDIR_CASE}/config.yaml"
  grep -q '^  - 192.168.2.11/32$' "${TMPDIR_CASE}/config.yaml"
  grep -q '^authentication:$' "${TMPDIR_CASE}/config.yaml"
  grep -q '^  - "alice:secret"$' "${TMPDIR_CASE}/config.yaml"
  grep -q '^  - "bob:pass"$' "${TMPDIR_CASE}/config.yaml"
  grep -q '^skip-auth-prefixes:$' "${TMPDIR_CASE}/config.yaml"
  grep -q '^  - 127.0.0.1/32$' "${TMPDIR_CASE}/config.yaml"
  grep -q '^  - 192.168.2.0/24$' "${TMPDIR_CASE}/config.yaml"
}

test_render_config_renders_external_ui_fields() {
  setup_case
  cat > "${TMPDIR_CASE}/settings.env" <<'EOF'
CONFIG_MODE="rule"
CORE_CHANNEL="alpha"
ALPHA_AUTO_UPDATE="0"
ALPHA_UPDATE_ONCALENDAR="daily"
RESTART_INTERVAL_HOURS="0"
RULESET_PRESET="default"
EXTERNAL_UI_NAME="metacubexd"
EXTERNAL_UI_URL="https://github.com/MetaCubeX/metacubexd/archive/refs/heads/gh-pages.zip"
EOF
  run_manager render-config >/dev/null
  grep -q '^external-ui-name: "metacubexd"$' "${TMPDIR_CASE}/config.yaml"
  grep -q '^external-ui-url: "https://github.com/MetaCubeX/metacubexd/archive/refs/heads/gh-pages.zip"$' "${TMPDIR_CASE}/config.yaml"
}

test_render_config_renders_controller_cors_fields() {
  setup_case
  cat > "${TMPDIR_CASE}/router.env" <<'EOF'
TEMPLATE_NAME="nas-single-lan-v4"
ENABLE_IPV6="0"
LAN_INTERFACES="bridge1"
LAN_CIDRS="192.168.2.0/24"
LAN_DISALLOWED_CIDRS=""
PROXY_INGRESS_INTERFACES="bridge1"
DNS_HIJACK_ENABLED="1"
DNS_HIJACK_INTERFACES="bridge1"
PROXY_AUTH_CREDENTIALS=""
SKIP_AUTH_PREFIXES=""
CONTROLLER_CORS_ALLOW_ORIGINS="http://192.168.2.10:3000 https://panel.example.com"
CONTROLLER_CORS_ALLOW_PRIVATE_NETWORK="1"
PROXY_HOST_OUTPUT="0"
BYPASS_CONTAINER_NAMES=""
BYPASS_SRC_CIDRS=""
BYPASS_DST_CIDRS=""
BYPASS_UIDS=""
MIXED_PORT="7890"
TPROXY_PORT="7893"
DNS_PORT="1053"
CONTROLLER_PORT="19090"
CONTROLLER_BIND_ADDRESS="0.0.0.0"
ROUTE_MARK="0x2333"
ROUTE_MASK="0xffffffff"
ROUTE_TABLE="233"
ROUTE_PRIORITY="100"
EOF
  run_manager render-config >/dev/null
  grep -q '^external-controller-cors:$' "${TMPDIR_CASE}/config.yaml"
  grep -q '^  allow-origins:$' "${TMPDIR_CASE}/config.yaml"
  grep -q '^    - "http://192.168.2.10:3000"$' "${TMPDIR_CASE}/config.yaml"
  grep -q '^    - "https://panel.example.com"$' "${TMPDIR_CASE}/config.yaml"
  grep -q '^  allow-private-network: true$' "${TMPDIR_CASE}/config.yaml"
}

test_default_rule_preset_is_rendered() {
  setup_case
  run_manager set-rule-preset default >/dev/null
  grep -q 'DOMAIN-SUFFIX,smzdm.com,DIRECT' "${TMPDIR_CASE}/ruleset/builtin.rules"
  grep -q 'DOMAIN,mtalk.google.com,PROXY' "${TMPDIR_CASE}/ruleset/builtin.rules"
  grep -q 'IP-CIDR,64.233.177.188/32,PROXY' "${TMPDIR_CASE}/ruleset/builtin.rules"
  grep -q 'DOMAIN-SUFFIX,smzdm.com,DIRECT' "${TMPDIR_CASE}/config.yaml"
  grep -q 'DOMAIN,mtalk.google.com,PROXY' "${TMPDIR_CASE}/config.yaml"
}

test_apply_default_template_command() {
  setup_case
  cat > "${TMPDIR_CASE}/settings.env" <<'EOF'
CONFIG_MODE="global"
RULESET_PRESET="unknown"
PROFILE_TEMPLATE="nas-single-lan-v4"
EOF
  sed -i 's/PROXY_HOST_OUTPUT="0"/PROXY_HOST_OUTPUT="1"/' "${TMPDIR_CASE}/router.env"
  sed -i 's/CONTROLLER_BIND_ADDRESS="127.0.0.1"/CONTROLLER_BIND_ADDRESS="0.0.0.0"/' "${TMPDIR_CASE}/router.env"
  run_manager apply-default-template >/dev/null
  grep -q '^CONFIG_MODE="rule"$' "${TMPDIR_CASE}/settings.env"
  grep -q '^RULESET_PRESET="default"$' "${TMPDIR_CASE}/settings.env"
  grep -q '^PROXY_HOST_OUTPUT="0"$' "${TMPDIR_CASE}/router.env"
  grep -q '^CONTROLLER_BIND_ADDRESS="0.0.0.0"$' "${TMPDIR_CASE}/router.env"
  grep -q 'DOMAIN-SUFFIX,smzdm.com,DIRECT' "${TMPDIR_CASE}/ruleset/builtin.rules"
}

test_rules_repo_command() {
  setup_case
  output="$(run_manager rules-repo)"
  assert_contains "$output" '规则仓库: '
  assert_contains "$output" 'rules-repo/default'
  assert_contains "$output" '- pt: type=domain_suffix target=direct'
  assert_contains "$output" '- fcm-site: type=domain target=proxy'
  assert_contains "$output" '- fcm-ip: type=ip_cidr target=proxy'
  assert_contains "$output" '总规则数: '
}

test_rules_repo_entries_command() {
  setup_case
  output="$(run_manager rules-repo-entries pt)"
  assert_contains "$output" 'ruleset=pt'
  assert_contains "$output" 'type=domain_suffix'
  assert_contains "$output" $'1\tsmzdm.com'
}

test_rules_repo_entries_command_with_keyword() {
  setup_case
  output="$(run_manager rules-repo-entries pt hd)"
  assert_contains "$output" 'ruleset=pt'
  assert_contains "$output" 'matched='
  assert_contains "$output" 'hdsky.me'
  if [[ "$output" == *$'\tsmzdm.com'* ]]; then
    echo "keyword filter should have excluded smzdm.com" >&2
    exit 1
  fi
}

test_rulepreset_describe_ruleset_command() {
  setup_case
  output="$(python3 "${RULEPRESETCTL}" describe-ruleset "${TMPDIR_CASE}/rules-repo/default/manifest.yaml" pt)"
  assert_contains "$output" 'ruleset=pt'
  assert_contains "$output" 'target=direct'
  assert_contains "$output" 'source=rules/direct/pt.txt'
}

test_rulepreset_search_entries_command() {
  setup_case
  output="$(python3 "${RULEPRESETCTL}" search-entries "${TMPDIR_CASE}/rules-repo/default/manifest.yaml" google)"
  assert_contains "$output" 'keyword=google'
  assert_contains "$output" $'fcm-site\tdomain\tproxy'
  assert_contains "$output" 'mtalk.google.com'
  assert_contains "$output" 'matched='
}

test_add_and_remove_repo_rule_commands() {
  setup_case
  run_manager add-repo-rule pt example-rule.test >/dev/null
  grep -q '^example-rule.test$' "${TMPDIR_CASE}/rules-repo/default/rules/direct/pt.txt"
  grep -q 'DOMAIN-SUFFIX,example-rule.test,DIRECT' "${TMPDIR_CASE}/ruleset/builtin.rules"
  grep -q 'DOMAIN-SUFFIX,example-rule.test,DIRECT' "${TMPDIR_CASE}/config.yaml"

  output="$(run_manager rules-repo-entries pt)"
  assert_contains "$output" $'example-rule.test'

  run_manager remove-repo-rule pt example-rule.test >/dev/null
  if grep -q '^example-rule.test$' "${TMPDIR_CASE}/rules-repo/default/rules/direct/pt.txt"; then
    echo "repo rule should have been removed" >&2
    exit 1
  fi
  if grep -q 'DOMAIN-SUFFIX,example-rule.test,DIRECT' "${TMPDIR_CASE}/ruleset/builtin.rules"; then
    echo "builtin rules should have been rerendered after removal" >&2
    exit 1
  fi
}

test_add_repo_rule_rejects_invalid_value() {
  setup_case
  if run_manager add-repo-rule fcm-ip not-an-ip >/tmp/mh-invalid-repo-rule.out 2>/tmp/mh-invalid-repo-rule.err; then
    echo "invalid repo rule should have failed" >&2
    exit 1
  fi
  grep -q 'invalid ip_cidr entry' /tmp/mh-invalid-repo-rule.err
  if grep -q '^not-an-ip$' "${TMPDIR_CASE}/rules-repo/default/rules/proxy/fcm-ip.txt"; then
    echo "invalid repo rule should not have been written" >&2
    exit 1
  fi
}

test_remove_repo_rule_by_index_command() {
  setup_case
  run_manager add-repo-rule pt index-delete.test >/dev/null
  output="$(run_manager rules-repo-entries pt)"
  index="$(printf '%s\n' "$output" | awk -F'\t' '$2 == "index-delete.test" {print $1; exit}')"
  [[ -n "$index" ]]

  run_manager remove-repo-rule-index pt "$index" >/dev/null
  if grep -q '^index-delete.test$' "${TMPDIR_CASE}/rules-repo/default/rules/direct/pt.txt"; then
    echo "repo rule should have been removed by index" >&2
    exit 1
  fi
  if grep -q 'DOMAIN-SUFFIX,index-delete.test,DIRECT' "${TMPDIR_CASE}/ruleset/builtin.rules"; then
    echo "builtin rules should have been rerendered after index removal" >&2
    exit 1
  fi
}

test_rules_repo_find_command() {
  setup_case
  output="$(run_manager rules-repo-find google)"
  assert_contains "$output" 'keyword=google'
  assert_contains "$output" 'mtalk.google.com'
  assert_contains "$output" 'matched='
}

test_status_readonly() {
  setup_case
  output="$(run_manager status)"
  assert_contains "$output" '当前模式: rule'
  assert_contains "$output" '当前模式来源: 本地配置回退'
  assert_contains "$output" '本地配置模式: rule'
  assert_contains "$output" '运行态策略组: 未获取'
  assert_contains "$output" '模板: nas-single-lan-v4 (单 LAN IPv4 旁路由)'
  assert_contains "$output" '规则预设: default (项目内置默认模板：PT 直连，FCM 域名/IP 强制代理)'
  assert_contains "$output" 'IPv6: 关闭'
  assert_contains "$output" '手动节点: 启用 0 / 总计 0'
  assert_contains "$output" '订阅: 启用 0 / 总计 0'
  assert_contains "$output" '订阅 provider: 启用 0 / 总计 0'
  assert_contains "$output" '订阅缓存: 就绪 0 / 总计 0'
  assert_contains "$output" '外部 UI 名称: 未设置'
  assert_contains "$output" '外部 UI 地址: 未设置'
  assert_contains "$output" '控制面 CORS Origins: 未设置'
  assert_contains "$output" '控制面 CORS Private-Network: 关闭'
  assert_contains "$output" '控制面运行态: 未获取'
  assert_contains "$output" 'WebUI: http://127.0.0.1:19090/ui/'
  assert_contains "$output" '局域网禁止网段: 无'
  assert_contains "$output" '显式代理认证: 关闭'
  assert_contains "$output" '显式代理免认证网段: 无'
  assert_contains "$output" '本机源码同步: 关闭'
  assert_contains "$output" 'Mixed/TProxy/DNS: 7890/7893/1053'
  assert_contains "$output" '宿主机流量: 默认直连；按需显式代理 http://127.0.0.1:7890'
  assert_contains "$output" '控制面密钥: 已隐藏；如需查看执行: mihomo show-secret'
  assert_contains "$output" '定时重启: 0h'
  assert_contains "$output" 'Alpha 自动更新: 关闭'
}

test_status_warns_on_host_output_proxy() {
  setup_case
  sed -i 's/PROXY_HOST_OUTPUT="0"/PROXY_HOST_OUTPUT="1"/' "${TMPDIR_CASE}/router.env"
  output="$(run_manager status)"
  assert_contains "$output" '宿主机流量: 透明接管(高风险)'
  assert_contains "$output" 'tailscaled、cloudflared'
}

test_status_warns_when_controller_exposed_to_lan() {
  setup_case
  sed -i 's/CONTROLLER_BIND_ADDRESS="127.0.0.1"/CONTROLLER_BIND_ADDRESS="0.0.0.0"/' "${TMPDIR_CASE}/router.env"
  output="$(run_manager status)"
  assert_contains "$output" '控制面当前已开放到局域网；更推荐保持 CONTROLLER_BIND_ADDRESS=127.0.0.1。'
}

test_status_recommends_import_when_no_nodes_or_subscriptions() {
  setup_case
  output="$(run_manager status)"
  assert_contains "$output" '推荐下一步: 导入节点: mihomo import-links 或添加订阅: mihomo add-subscription'
}

test_status_recommends_update_subscriptions_when_provider_not_ready() {
  setup_case
  run_manager add-subscription demo https://subscription.example/list.txt 1 >/dev/null
  output="$(run_manager status)"
  assert_contains "$output" '推荐下一步: 更新订阅: mihomo update-subscriptions'
}

test_status_recommends_start_when_nodes_ready_but_service_inactive() {
  setup_case
  python3 "${STATECTL}" append-node "${TMPDIR_CASE}/state/nodes.json" 'vless://uuid@example.com:443?encryption=none&security=reality&sni=www.microsoft.com&fp=chrome&pbk=PUBLIC_KEY&sid=abcd&type=tcp#manual-node' manual-node 1 >/dev/null
  output="$(run_manager status)"
  assert_contains "$output" '推荐下一步: 启动服务: mihomo start'
}

test_status_shows_official_access_fields() {
  setup_case
  sed -i 's/PROXY_HOST_OUTPUT="0"/PROXY_HOST_OUTPUT="0"\nLAN_DISALLOWED_CIDRS="192.168.2.10\/32"\nPROXY_AUTH_CREDENTIALS="alice:secret bob:pass"\nSKIP_AUTH_PREFIXES="127.0.0.1\/32"/' "${TMPDIR_CASE}/router.env"
  output="$(run_manager status)"
  assert_contains "$output" '局域网禁止网段: 192.168.2.10/32'
  assert_contains "$output" '显式代理认证: 启用 (2 组账号)'
  assert_contains "$output" '显式代理免认证网段: 127.0.0.1/32'
}

test_status_shows_external_ui_fields() {
  setup_case
  cat > "${TMPDIR_CASE}/settings.env" <<'EOF'
CONFIG_MODE="rule"
CORE_CHANNEL="alpha"
ALPHA_AUTO_UPDATE="0"
ALPHA_UPDATE_ONCALENDAR="daily"
RESTART_INTERVAL_HOURS="0"
RULESET_PRESET="default"
EXTERNAL_UI_NAME="metacubexd"
EXTERNAL_UI_URL="https://github.com/MetaCubeX/metacubexd/archive/refs/heads/gh-pages.zip"
EOF
  output="$(run_manager status)"
  assert_contains "$output" '外部 UI 名称: metacubexd'
  assert_contains "$output" '外部 UI 地址: https://github.com/MetaCubeX/metacubexd/archive/refs/heads/gh-pages.zip'
}

test_status_shows_controller_cors_fields() {
  setup_case
  sed -i 's/PROXY_HOST_OUTPUT="0"/PROXY_HOST_OUTPUT="0"\nCONTROLLER_CORS_ALLOW_ORIGINS="http:\/\/192.168.2.10:3000 https:\/\/panel.example.com"\nCONTROLLER_CORS_ALLOW_PRIVATE_NETWORK="1"/' "${TMPDIR_CASE}/router.env"
  output="$(run_manager status)"
  assert_contains "$output" '控制面 CORS Origins: http://192.168.2.10:3000 https://panel.example.com'
  assert_contains "$output" '控制面 CORS Private-Network: 启用'
}

test_templates_mark_dualstack_as_deprecated() {
  setup_case
  output="$(run_manager templates)"
  assert_contains "$output" 'nas-single-lan-dualstack - 双栈模板占位（未实现真双栈旁路由）'
}

test_status_warns_on_dualstack_placeholder() {
  setup_case
  sed -i 's/TEMPLATE_NAME="nas-single-lan-v4"/TEMPLATE_NAME="nas-single-lan-dualstack"/' "${TMPDIR_CASE}/router.env"
  output="$(run_manager status)"
  assert_contains "$output" '模板: nas-single-lan-dualstack (双栈模板占位（未实现真双栈旁路由）)'
  assert_contains "$output" '当前模板仅兼容保留；本项目当前只承诺 Debian NAS 的 IPv4 旁路由。'
}

test_render_config_uses_subscription_provider_cache() {
  setup_case
  run_manager add-subscription demo https://subscription.example/list.txt 1 >/dev/null
  sub_id="$(python3 "${STATECTL}" list-subscriptions "${TMPDIR_CASE}/state/subscriptions.json" | awk -F'\t' 'NR==1{print $2}')"
  mkdir -p "${TMPDIR_CASE}/proxy_providers/subscriptions"
  cat > "${TMPDIR_CASE}/proxy_providers/subscriptions/${sub_id}.txt" <<'EOF'
vless://uuid@example.com:443?encryption=none&security=reality&sni=www.microsoft.com&fp=chrome&pbk=PUBLIC_KEY&sid=abcd&type=tcp#sub-provider-node
EOF
  python3 "${STATECTL}" append-node "${TMPDIR_CASE}/state/nodes.json" 'vless://uuid@example.com:443?encryption=none&security=reality&sni=www.microsoft.com&fp=chrome&pbk=PUBLIC_KEY&sid=abcd&type=tcp#manual-node' manual-node 1 >/dev/null
  python3 "${STATECTL}" append-node "${TMPDIR_CASE}/state/nodes.json" 'trojan://password@example.org:443?security=tls&sni=www.apple.com&type=ws&host=www.apple.com&path=%2Fws#sub-node-cache' sub-node-cache 1 subscription "${sub_id}" >/dev/null
  run_manager render-config >/dev/null
  grep -q 'name: "manual-node"' "${TMPDIR_CASE}/proxy_providers/manual.txt"
  if grep -q 'sub-node-cache' "${TMPDIR_CASE}/proxy_providers/manual.txt"; then
    echo "subscription cache nodes should not be rendered into manual provider" >&2
    exit 1
  fi
  grep -q "./proxy_providers/subscriptions/${sub_id}.txt" "${TMPDIR_CASE}/config.yaml"
  grep -q "subscription-${sub_id%%-*}:" "${TMPDIR_CASE}/config.yaml"
}

test_rule_targets_reject_subscription_cache_nodes() {
  setup_case
  python3 "${STATECTL}" append-node "${TMPDIR_CASE}/state/nodes.json" 'trojan://password@example.org:443?security=tls&sni=www.apple.com&type=ws&host=www.apple.com&path=%2Fws#sub-node-cache' sub-node-cache 1 subscription sub-001 >/dev/null
  python3 "${STATECTL}" add-rule "${TMPDIR_CASE}/state/acl.json" port 443 sub-node-cache >/dev/null
  if run_manager render-config >/tmp/mh-sub-target.out 2>/tmp/mh-sub-target.err; then
    echo "subscription cache node should not be accepted as rule target" >&2
    exit 1
  fi
  grep -q 'ACL 规则存在指向不存在或未启用节点的目标' /tmp/mh-sub-target.err
}

test_status_distinguishes_manual_nodes_and_subscription_cache() {
  setup_case
  run_manager add-subscription demo https://subscription.example/list.txt 1 >/dev/null
  sub_id="$(python3 "${STATECTL}" list-subscriptions "${TMPDIR_CASE}/state/subscriptions.json" | awk -F'\t' 'NR==1{print $2}')"
  mkdir -p "${TMPDIR_CASE}/proxy_providers/subscriptions"
  cat > "${TMPDIR_CASE}/proxy_providers/subscriptions/${sub_id}.txt" <<'EOF'
vless://uuid@example.com:443?encryption=none&security=reality&sni=www.microsoft.com&fp=chrome&pbk=PUBLIC_KEY&sid=abcd&type=tcp#sub-provider-node
EOF
  python3 "${STATECTL}" append-node "${TMPDIR_CASE}/state/nodes.json" 'vless://uuid@example.com:443?encryption=none&security=reality&sni=www.microsoft.com&fp=chrome&pbk=PUBLIC_KEY&sid=abcd&type=tcp#manual-node' manual-node 1 >/dev/null
  python3 "${STATECTL}" append-node "${TMPDIR_CASE}/state/nodes.json" 'trojan://password@example.org:443?security=tls&sni=www.apple.com&type=ws&host=www.apple.com&path=%2Fws#sub-node-cache' sub-node-cache 1 subscription "${sub_id}" >/dev/null
  python3 "${STATECTL}" mark-subscription-success "${TMPDIR_CASE}/state/subscriptions.json" "${sub_id}" 1 >/dev/null
  output="$(run_manager status)"
  assert_contains "$output" '手动节点: 启用 1 / 总计 1'
  assert_contains "$output" '订阅: 启用 1 / 总计 1'
  assert_contains "$output" '订阅 provider: 启用 1 / 总计 1'
  assert_contains "$output" '订阅缓存: 就绪 1 / 总计 1'
}

test_subscription_nodes_are_readonly() {
  setup_case
  python3 "${STATECTL}" append-node "${TMPDIR_CASE}/state/nodes.json" 'trojan://password@example.org:443?security=tls&sni=www.apple.com&type=ws&host=www.apple.com&path=%2Fws#sub-node-cache' sub-node-cache 1 subscription sub-001 >/dev/null
  if python3 "${STATECTL}" rename-node "${TMPDIR_CASE}/state/nodes.json" 1 renamed-node "${TMPDIR_CASE}/state/rules.json" >/tmp/mh-sub-rename.out 2>/tmp/mh-sub-rename.err; then
    echo "subscription node rename should fail" >&2
    exit 1
  fi
  grep -q 'provider-managed' /tmp/mh-sub-rename.err
  if python3 "${STATECTL}" set-node-enabled "${TMPDIR_CASE}/state/nodes.json" 1 0 >/tmp/mh-sub-toggle.out 2>/tmp/mh-sub-toggle.err; then
    echo "subscription node toggle should fail" >&2
    exit 1
  fi
  grep -q 'provider-managed' /tmp/mh-sub-toggle.err
}

test_nodes_list_hides_subscription_cache_nodes() {
  setup_case
  python3 "${STATECTL}" append-node "${TMPDIR_CASE}/state/nodes.json" 'trojan://password@example.org:443?security=tls&sni=www.apple.com&type=ws&host=www.apple.com&path=%2Fws#sub-node-cache' sub-node-cache 1 subscription sub-001 >/dev/null
  python3 "${STATECTL}" append-node "${TMPDIR_CASE}/state/nodes.json" 'vless://uuid@example.com:443?encryption=none&security=reality&sni=www.microsoft.com&fp=chrome&pbk=PUBLIC_KEY&sid=abcd&type=tcp#manual-node' manual-node 1 >/dev/null
  output="$(run_manager nodes)"
  assert_contains "$output" 'manual-node'
  if [[ "$output" == *'sub-node-cache'* ]]; then
    echo "subscription cache nodes should not appear in nodes list" >&2
    exit 1
  fi
}

test_usage_mentions_new_commands() {
  output="$(run_manager help)"
  assert_contains "$output" 'apply-default-template'
  assert_contains "$output" 'rules-repo-entries'
  assert_contains "$output" 'rules-repo-find'
  assert_contains "$output" 'rules-repo-entries [ruleset] [keyword]'
  assert_contains "$output" 'add-repo-rule'
  assert_contains "$output" 'remove-repo-rule'
  assert_contains "$output" 'remove-repo-rule-index'
  assert_contains "$output" 'repair'
  assert_contains "$output" 'install-self-sync [minutes]'
  assert_contains "$output" 'disable-self-sync'
  assert_contains "$output" 'install-webui [name] [url]'
  assert_contains "$output" 'templates'
  assert_contains "$output" 'rules-repo'
  assert_contains "$output" 'rule-presets'
  assert_contains "$output" 'update-subscriptions'
  assert_contains "$output" 'rollback-config'
  assert_contains "$output" '兼容命令:'
}

test_menu_mentions_new_buckets() {
  grep -q 'echo "3) 节点与订阅"' "${ROOT}/mihomo"
  grep -q 'echo "4) 网络入口与模板"' "${ROOT}/mihomo"
  grep -q 'echo "5) 访问控制 ACL"' "${ROOT}/mihomo"
  grep -q 'echo "8) 回滚与诊断"' "${ROOT}/mihomo"
}

main() {
  test_syntax
  test_render_empty
  test_protocol_renderers
  test_acl_rules_are_rendered
  test_auto_without_node_fails
  test_scan_marks_unsupported_scheme
  test_scan_marks_invalid_vmess_payload
  test_scan_marks_invalid_ss_payload
  test_scan_marks_invalid_vless_port
  test_subscription_state_commands
  test_subscription_state_uses_cache_and_enumeration_subobjects
  test_config_loader_treats_values_as_literals
  test_render_config_renders_official_access_fields
  test_render_config_renders_external_ui_fields
  test_render_config_renders_controller_cors_fields
  test_default_rule_preset_is_rendered
  test_apply_default_template_command
  test_rules_repo_command
  test_rules_repo_entries_command
  test_rules_repo_entries_command_with_keyword
  test_rulepreset_describe_ruleset_command
  test_rulepreset_search_entries_command
  test_add_and_remove_repo_rule_commands
  test_add_repo_rule_rejects_invalid_value
  test_remove_repo_rule_by_index_command
  test_rules_repo_find_command
  test_status_readonly
  test_status_warns_on_host_output_proxy
  test_status_shows_official_access_fields
  test_status_shows_external_ui_fields
  test_status_shows_controller_cors_fields
  test_templates_mark_dualstack_as_deprecated
  test_status_warns_on_dualstack_placeholder
  test_render_config_uses_subscription_provider_cache
  test_rule_targets_reject_subscription_cache_nodes
  test_status_distinguishes_manual_nodes_and_subscription_cache
  test_subscription_nodes_are_readonly
  test_nodes_list_hides_subscription_cache_nodes
  test_usage_mentions_new_commands
  test_menu_mentions_new_buckets
  echo "smoke: ok"
}

main "$@"
