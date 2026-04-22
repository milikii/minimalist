#!/usr/bin/env bash

set -euo pipefail

ROOT="$(cd "$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")/.." && pwd)"
MANAGER="${ROOT}/mihomo"
STATECTL="${ROOT}/scripts/statectl.py"

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
  cat > "${TMPDIR_CASE}/router.env" <<'EOF'
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
ROUTE_MARK="0x2333"
ROUTE_MASK="0xffffffff"
ROUTE_TABLE="233"
ROUTE_PRIORITY="100"
EOF
}

env_prefix() {
  printf 'APP_ROOT=%q MIHOMO_DIR=%q SETTINGS_ENV=%q ROUTER_ENV=%q CONFIG_FILE=%q RULES_DIR=%q PROVIDER_DIR=%q UI_DIR=%q STATE_DIR=%q NODES_STATE_FILE=%q RULES_STATE_FILE=%q PROVIDER_FILE=%q RENDERED_RULES_FILE=%q MIHOMO_USER=%q MANAGER_BIN=%q MIHOMO_BIN=%q' \
    "$ROOT" \
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
    "$TMPDIR_CASE/proxy_providers/manual.txt" \
    "$TMPDIR_CASE/ruleset/custom.rules" \
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
}

test_render_empty() {
  setup_case
  run_manager render-config >/dev/null
  grep -q '^proxies: \[\]' "${TMPDIR_CASE}/proxy_providers/manual.txt"
  grep -q '^  - 192.168.2.0/24' "${TMPDIR_CASE}/config.yaml"
  grep -q '^  - 127.0.0.0/8' "${TMPDIR_CASE}/config.yaml"
  grep -q '^  - MATCH,PROXY' "${TMPDIR_CASE}/config.yaml"
}

test_rename_rule_sync() {
  setup_case
  run_manager render-config >/dev/null
  python3 "${STATECTL}" append-node "${TMPDIR_CASE}/state/nodes.json" 'vless://uuid@example.com:443?encryption=none&security=reality&sni=www.microsoft.com&fp=chrome&pbk=PUBLIC_KEY&sid=abcd&type=tcp&flow=xtls-rprx-vision#Old-Name' Old-Name 1 >/dev/null
  python3 "${STATECTL}" add-rule "${TMPDIR_CASE}/state/rules.json" domain foo.com Old-Name >/dev/null
  python3 "${STATECTL}" rename-node "${TMPDIR_CASE}/state/nodes.json" 1 New-Name "${TMPDIR_CASE}/state/rules.json" >/dev/null
  run_manager render-config >/dev/null
  grep -q 'DOMAIN,foo.com,New-Name' "${TMPDIR_CASE}/ruleset/custom.rules"
  grep -q '#New-Name' "${TMPDIR_CASE}/proxy_providers/manual.txt"
}

test_auto_without_node_fails() {
  setup_case
  run_manager render-config >/dev/null
  python3 "${STATECTL}" add-rule "${TMPDIR_CASE}/state/rules.json" domain foo.com AUTO >/dev/null
  if run_manager render-config >/tmp/mh-smoke-auto.log 2>&1; then
    echo "AUTO target should have failed without enabled nodes" >&2
    exit 1
  fi
  grep -q '存在自定义规则指向不存在或未启用节点' /tmp/mh-smoke-auto.log
}

test_disabled_legacy_migration() {
  setup_case
  mkdir -p "${TMPDIR_CASE}/proxy_providers" "${TMPDIR_CASE}/state"
  printf '%s\n' '#DISABLED#vless://uuid@example.com:443?encryption=none&security=tls#old-disabled' 'vless://uuid2@example.org:443?encryption=none&security=tls#old-enabled' > "${TMPDIR_CASE}/proxy_providers/manual.txt"
  python3 "${STATECTL}" ensure-nodes-state "${TMPDIR_CASE}/state/nodes.json" "${TMPDIR_CASE}/proxy_providers/manual.txt" >/dev/null
  local output
  output="$(cat "${TMPDIR_CASE}/state/nodes.json")"
  assert_contains "$output" '"name": "old-disabled"'
  assert_contains "$output" '"enabled": false'
  assert_contains "$output" '"name": "old-enabled"'
}

test_status_readonly() {
  setup_case
  output="$(run_manager status)"
  [[ ! -e "${TMPDIR_CASE}/state/nodes.json" ]]
  assert_contains "$output" '节点: 启用 0 / 总计 0'
  assert_contains "$output" '宿主机流量接管: 关闭(推荐)'
}

test_status_warns_on_host_output_proxy() {
  setup_case
  sed -i 's/PROXY_HOST_OUTPUT="0"/PROXY_HOST_OUTPUT="1"/' "${TMPDIR_CASE}/router.env"
  output="$(run_manager status)"
  assert_contains "$output" '宿主机流量接管: 开启(高风险)'
  assert_contains "$output" 'tailscaled、cloudflared'
}

test_reply_bypass_present() {
  grep -q -- '--ctdir REPLY -j RETURN' "${ROOT}/lib/render.sh"
}

test_legacy_host_dns_cleanup_present() {
  grep -q 'delete_jump nat OUTPUT -j MIHOMO_DNS_OUT' "${ROOT}/lib/render.sh"
  grep -q 'ipt -t nat -F MIHOMO_DNS_OUT' "${ROOT}/lib/render.sh"
}

test_host_output_conflict_guard() {
  setup_case
  sed -i 's/PROXY_HOST_OUTPUT="0"/PROXY_HOST_OUTPUT="1"/' "${TMPDIR_CASE}/router.env"
  mkdir -p "${TMPDIR_CASE}/bin"
  cat > "${TMPDIR_CASE}/bin/systemctl" <<'EOF'
#!/usr/bin/env bash
if [[ "$1" == "is-active" && "$2" == "--quiet" ]]; then
  case " ${ACTIVE_UNITS:-} " in
    *" ${3} "*) exit 0 ;;
  esac
fi
exit 1
EOF
  chmod +x "${TMPDIR_CASE}/bin/systemctl"
  if APP_ROOT="$ROOT" MIHOMO_DIR="$TMPDIR_CASE" ROUTER_ENV="${TMPDIR_CASE}/router.env" SYSTEMCTL_BIN="${TMPDIR_CASE}/bin/systemctl" ACTIVE_UNITS="tailscaled.service cloudflared.service" bash -lc 'source "$APP_ROOT/lib/common.sh"; load_router_env; guard_host_output_proxy_conflicts' >/tmp/mh-host-output-guard.log 2>&1; then
    echo 'host output conflict guard should have failed' >&2
    exit 1
  fi
  grep -q 'tailscaled cloudflared' /tmp/mh-host-output-guard.log
  grep -q 'PROXY_HOST_OUTPUT=0' /tmp/mh-host-output-guard.log
}

test_safe_host_output_default_present() {
  grep -q 'PROXY_HOST_OUTPUT="0"' "${ROOT}/lib/common.sh"
}

main() {
  test_syntax
  test_render_empty
  test_rename_rule_sync
  test_auto_without_node_fails
  test_disabled_legacy_migration
  test_status_readonly
  test_status_warns_on_host_output_proxy
  test_reply_bypass_present
  test_legacy_host_dns_cleanup_present
  test_host_output_conflict_guard
  test_safe_host_output_default_present
  echo "smoke: ok"
}

main "$@"
