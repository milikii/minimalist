#!/usr/bin/env bash

set -euo pipefail

ROOT="$(cd "$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")/.." && pwd)"
MANAGER="${ROOT}/mihomo"

cleanup() {
  [[ -n "${TMPDIR_CASE:-}" && -d "${TMPDIR_CASE:-}" ]] && rm -rf "$TMPDIR_CASE"
}
trap cleanup EXIT

setup_case() {
  TMPDIR_CASE="$(mktemp -d)"
  mkdir -p "${TMPDIR_CASE}/bin" "${TMPDIR_CASE}/state" "${TMPDIR_CASE}/ruleset" "${TMPDIR_CASE}/proxy_providers" "${TMPDIR_CASE}/ui"
  cp -a "${ROOT}/rules-repo" "${TMPDIR_CASE}/rules-repo"

  cat > "${TMPDIR_CASE}/router.env" <<'EOENV'
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
EOENV

  cat > "${TMPDIR_CASE}/bin/systemctl" <<'EOSYS'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "${SYSTEMCTL_LOG:?}"
case "$1" in
  show)
    prop="$4"
    case "$prop" in
      ActiveState) echo "active" ;;
      SubState) echo "running" ;;
      MainPID) echo "12345" ;;
      ActiveEnterTimestamp) echo "Mon 2026-04-20 12:00:00 CST" ;;
      NRestarts) echo "0" ;;
      MemoryCurrent) echo "10485760" ;;
      MemoryPeak) echo "20971520" ;;
      CPUUsageNSec) echo "123456789" ;;
      NextElapseUSecRealtime) echo "Tue 2026-04-21 00:00:00 CST" ;;
      *) echo "" ;;
    esac
    exit 0
    ;;
  is-active|is-enabled)
    unit="$2"
    if [[ "$unit" == "--quiet" ]]; then
      unit="$3"
    fi
    case "$unit" in
      mihomo|mihomo-alpha-update.timer) exit 0 ;;
      mihomo-restart.timer) exit 1 ;;
      *) exit 1 ;;
    esac
    ;;
  *)
    exit 0
    ;;
esac
EOSYS
  chmod +x "${TMPDIR_CASE}/bin/systemctl"

  cat > "${TMPDIR_CASE}/bin/journalctl" <<'EOJ'
#!/usr/bin/env bash
if printf '%s\n' "$*" | grep -q -- '--since 24 hours ago'; then
  case "$*" in
    *'-p warning'*) printf 'warning line\n'; exit 0 ;;
    *'-p err'*) exit 0 ;;
  esac
fi
printf 'journal output\n'
EOJ
  chmod +x "${TMPDIR_CASE}/bin/journalctl"

  cat > "${TMPDIR_CASE}/bin/ss" <<'EOSS'
#!/usr/bin/env bash
cat <<OUT
tcp LISTEN 0 4096 *:7890 *:* users:(("mihomo-core",pid=12345,fd=10))
tcp LISTEN 0 4096 *:7893 *:* users:(("mihomo-core",pid=12345,fd=7))
tcp LISTEN 0 4096 *:1053 *:* users:(("mihomo-core",pid=12345,fd=11))
tcp LISTEN 0 4096 *:19090 *:* users:(("mihomo-core",pid=12345,fd=6))
OUT
EOSS
  chmod +x "${TMPDIR_CASE}/bin/ss"

  cat > "${TMPDIR_CASE}/bin/iptables" <<'EOIPT'
#!/usr/bin/env bash
if [[ "$1" == "-t" && "$2" == "mangle" && "$3" == "-L" && "$4" == "MIHOMO_PRE_HANDLE" ]]; then
  cat <<OUT
Chain MIHOMO_PRE_HANDLE (1 references)
 pkts bytes target     prot opt in     out     source               destination
   42  4200 TPROXY     tcp  --  *      *       0.0.0.0/0            0.0.0.0/0
   24  2400 TPROXY     udp  --  *      *       0.0.0.0/0            0.0.0.0/0
OUT
  exit 0
fi
if [[ "$1" == "-t" && "$2" == "nat" && "$3" == "-L" && "$4" == "MIHOMO_DNS_HANDLE" ]]; then
  cat <<OUT
Chain MIHOMO_DNS_HANDLE (1 references)
 pkts bytes target     prot opt in     out     source               destination
   12  1200 REDIRECT   udp  --  *      *       0.0.0.0/0            0.0.0.0/0
    6   600 REDIRECT   tcp  --  *      *       0.0.0.0/0            0.0.0.0/0
OUT
  exit 0
fi
exit 0
EOIPT
  chmod +x "${TMPDIR_CASE}/bin/iptables"

  cat > "${TMPDIR_CASE}/bin/curl" <<'EOCURL'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "${CURL_LOG:?}"
out=""
target=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    -o)
      out="$2"
      shift 2
      ;;
    http*)
      target="$1"
      shift
      ;;
    *)
      shift
      ;;
  esac
done

if [[ "$target" == *"geosite.dat"* ]]; then
  printf 'mock-geosite-data\n' > "$out"
  exit 0
fi

if [[ "$target" == *"country.mmdb"* ]]; then
  printf 'mock-country-mmdb\n' > "$out"
  exit 0
fi

if [[ "$target" == *"gh-pages.zip"* ]]; then
  printf '<!doctype html>\n' > "$out"
  exit 0
fi

if [[ "$target" == *"subscription.example"* ]]; then
  cat > "$out" <<OUT
vless://uuid@example.com:443?encryption=none&security=reality&sni=www.microsoft.com&fp=chrome&pbk=PUBLIC_KEY&sid=abcd&type=tcp#sub-vless
trojan://password@example.org:443?security=tls&sni=www.apple.com&type=ws&host=www.apple.com&path=%2Fws#sub-trojan
OUT
  exit 0
fi

if [[ "$target" == *"/configs" ]]; then
  if [[ -n "${CONTROLLER_CONFIGS_JSON_FILE:-}" && -f "${CONTROLLER_CONFIGS_JSON_FILE}" ]]; then
    if [[ -n "$out" ]]; then
      cat "${CONTROLLER_CONFIGS_JSON_FILE}" > "$out"
    else
      cat "${CONTROLLER_CONFIGS_JSON_FILE}"
    fi
    exit 0
  fi
  exit 22
fi

if [[ "$target" == *"/proxies" ]]; then
  if [[ -n "${CONTROLLER_PROXIES_JSON_FILE:-}" && -f "${CONTROLLER_PROXIES_JSON_FILE}" ]]; then
    if [[ -n "$out" ]]; then
      cat "${CONTROLLER_PROXIES_JSON_FILE}" > "$out"
    else
      cat "${CONTROLLER_PROXIES_JSON_FILE}"
    fi
    exit 0
  fi
  exit 22
fi

if [[ "$target" == *"/version" ]]; then
  if [[ -n "${CONTROLLER_VERSION_JSON_FILE:-}" && -f "${CONTROLLER_VERSION_JSON_FILE}" ]]; then
    if [[ -n "$out" ]]; then
      cat "${CONTROLLER_VERSION_JSON_FILE}" > "$out"
    else
      cat "${CONTROLLER_VERSION_JSON_FILE}"
    fi
    exit 0
  fi
  exit 22
fi

if [[ -n "$out" ]]; then
  printf '<!doctype html>\n' > "$out"
else
  printf '<!doctype html>\n'
fi
EOCURL
  chmod +x "${TMPDIR_CASE}/bin/curl"

  cat > "${TMPDIR_CASE}/bin/unzip" <<'EOUNZIP'
#!/usr/bin/env bash
if [[ -z "${UNZIP_OK_FLAG:-}" || ! -f "${UNZIP_OK_FLAG}" ]]; then
  exit 1
fi
dest=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    -d)
      dest="$2"
      shift 2
      ;;
    *)
      shift
      ;;
  esac
done
mkdir -p "${dest:?}/mock-ui"
printf '<!doctype html>\n' > "${dest}/mock-ui/index.html"
EOUNZIP
  chmod +x "${TMPDIR_CASE}/bin/unzip"

}

install_pipefail_ss_mock() {
  cat > "${TMPDIR_CASE}/bin/ss" <<'EOSS'
#!/usr/bin/env bash
printf 'tcp LISTEN 0 4096 *:7890 *:* users:(("mihomo-core",pid=12345,fd=10))\n'
printf 'tcp LISTEN 0 4096 *:7893 *:* users:(("mihomo-core",pid=12345,fd=7))\n'
printf 'tcp LISTEN 0 4096 *:1053 *:* users:(("mihomo-core",pid=12345,fd=11))\n'
printf 'tcp LISTEN 0 4096 *:19090 *:* users:(("mihomo-core",pid=12345,fd=6))\n'

for _ in $(seq 1 32); do
  printf 'tcp LISTEN 0 4096 *:65535 *:* users:(("other",pid=1,fd=1))\n' || exit 141
done
EOSS
  chmod +x "${TMPDIR_CASE}/bin/ss"
}

env_prefix() {
  printf 'PATH=%q UNZIP_OK_FLAG=%q APP_ROOT=%q INSTALL_ROOT=%q RULE_REPO_ROOT=%q MIHOMO_DIR=%q SETTINGS_ENV=%q ROUTER_ENV=%q CONFIG_FILE=%q RULES_DIR=%q PROVIDER_DIR=%q UI_DIR=%q STATE_DIR=%q NODES_STATE_FILE=%q RULES_STATE_FILE=%q ACL_STATE_FILE=%q SUBSCRIPTIONS_STATE_FILE=%q PROVIDER_FILE=%q RENDERED_RULES_FILE=%q ACL_RENDERED_RULES_FILE=%q MIHOMO_USER=%q MANAGER_BIN=%q COMPAT_MANAGER_BIN=%q MIHOMO_BIN=%q SYSTEMCTL_BIN=%q JOURNALCTL_BIN=%q SS_BIN=%q CURL_BIN=%q IPTABLES_BIN=%q CONTROLLER_CONFIGS_JSON_FILE=%q CONTROLLER_PROXIES_JSON_FILE=%q CONTROLLER_VERSION_JSON_FILE=%q SYSTEMCTL_LOG=%q CURL_LOG=%q SYSTEMD_UNIT=%q RESTART_SERVICE_UNIT=%q RESTART_TIMER_UNIT=%q UPDATE_SERVICE_UNIT=%q UPDATE_TIMER_UNIT=%q MANAGER_SYNC_SERVICE_UNIT=%q MANAGER_SYNC_TIMER_UNIT=%q ROUTER_SYSCTL=%q' \
    "${TMPDIR_CASE}/bin:${PATH}" \
    "${TMPDIR_CASE}/unzip-ok" \
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
    "$TMPDIR_CASE/mihomo-sidecar.sh" \
    /bin/true \
    "$TMPDIR_CASE/bin/systemctl" \
    "$TMPDIR_CASE/bin/journalctl" \
    "$TMPDIR_CASE/bin/ss" \
    "$TMPDIR_CASE/bin/curl" \
    "$TMPDIR_CASE/bin/iptables" \
    "${TMPDIR_CASE}/controller-configs.json" \
    "${TMPDIR_CASE}/controller-proxies.json" \
    "${TMPDIR_CASE}/controller-version.json" \
    "$TMPDIR_CASE/systemctl.log" \
    "$TMPDIR_CASE/curl.log" \
    "$TMPDIR_CASE/mihomo.service" \
    "$TMPDIR_CASE/mihomo-restart.service" \
    "$TMPDIR_CASE/mihomo-restart.timer" \
    "$TMPDIR_CASE/mihomo-alpha-update.service" \
    "$TMPDIR_CASE/mihomo-alpha-update.timer" \
    "$TMPDIR_CASE/mihomo-manager-sync.service" \
    "$TMPDIR_CASE/mihomo-manager-sync.timer" \
    "$TMPDIR_CASE/99-mihomo-router.conf"
}

run_manager() {
  local cmd
  cmd="$(env_prefix)"
  # shellcheck disable=SC2086
  eval "$cmd" "$MANAGER" "$@"
}

assert_log_contains() {
  local needle="$1"
  grep -Fq "$needle" "${TMPDIR_CASE}/systemctl.log"
}

test_start_without_nodes_fails_before_systemctl_start() {
  setup_case
  run_manager render-config >/dev/null
  if run_manager start >/tmp/mh-service-start.log 2>&1; then
    echo "start should fail without nodes" >&2
    exit 1
  fi
  grep -q '当前没有启用中的节点' /tmp/mh-service-start.log
  [[ ! -f "${TMPDIR_CASE}/systemctl.log" ]] || ! grep -Fq 'start mihomo' "${TMPDIR_CASE}/systemctl.log"
}

test_configure_restart_enables_timer() {
  setup_case
  run_manager configure-restart 24 >/dev/null
  assert_log_contains 'daemon-reload'
  assert_log_contains 'enable --now mihomo-restart.timer'
}

test_disable_alpha_update_disables_timer() {
  setup_case
  run_manager configure-alpha-update 0 daily >/dev/null
  assert_log_contains 'disable --now mihomo-alpha-update.timer'
}

test_runtime_audit_outputs() {
  setup_case
  run_manager render-config >/dev/null
  output="$(run_manager runtime-audit)"
  grep -q '服务状态: active' <<<"$output"
  grep -q '当前模式: rule' <<<"$output"
  grep -q '当前模式来源: 本地配置回退' <<<"$output"
  grep -q '本地配置模式: rule' <<<"$output"
  grep -q '运行态策略组: 未获取' <<<"$output"
  grep -q '控制面运行态: 未获取' <<<"$output"
  grep -q '当前模板: nas-single-lan-v4 (单 LAN IPv4 旁路由)' <<<"$output"
  grep -q '过去 24 小时 warning 数: 1' <<<"$output"
  grep -q '下次 Alpha 自动更新: Tue 2026-04-21 00:00:00 CST' <<<"$output"
  grep -q '外部 UI 名称: 未设置' <<<"$output"
  grep -q '外部 UI 地址: 未设置' <<<"$output"
  grep -q '控制面 CORS Origins: 未设置' <<<"$output"
  grep -q '控制面 CORS Private-Network: 关闭' <<<"$output"
  grep -q '控制面范围: 仅宿主机' <<<"$output"
  grep -q '局域网网段: 192.168.2.0/24' <<<"$output"
  grep -q '局域网禁止网段: 无' <<<"$output"
  grep -q 'DNS 劫持入口: bridge1' <<<"$output"
  grep -q '显式代理认证: 关闭' <<<"$output"
  grep -q '显式代理免认证网段: 无' <<<"$output"
  grep -q 'localhost 显式代理探测: ok' <<<"$output"
  grep -q '局域网透明代理命中包数: 66' <<<"$output"
  grep -q 'DNS 劫持命中包数: 18' <<<"$output"
}

test_runtime_audit_reads_mode_from_controller() {
  setup_case
  run_manager render-config >/dev/null
  cat > "${TMPDIR_CASE}/controller-configs.json" <<'EOF'
{"mode":"global"}
EOF
  cat > "${TMPDIR_CASE}/controller-proxies.json" <<'EOF'
{"proxies":{"PROXY":{"type":"Selector","now":"AUTO","all":["DIRECT","AUTO"]},"AUTO":{"type":"URLTest","now":"manual-node","all":["manual-node"]}}}
EOF
  cat > "${TMPDIR_CASE}/controller-version.json" <<'EOF'
{"meta":true,"version":"Mihomo Meta v1.19.10"}
EOF
  output="$(run_manager runtime-audit)"
  grep -q '当前模式: global' <<<"$output"
  grep -q '当前模式来源: Mihomo REST API' <<<"$output"
  grep -q '本地配置模式: rule' <<<"$output"
  grep -q '运行态策略组: PROXY=AUTO; AUTO=manual-node' <<<"$output"
  grep -q '控制面运行态: API 可达; 版本 Mihomo Meta v1.19.10' <<<"$output"
  grep -Fq 'http://127.0.0.1:19090/configs' "${TMPDIR_CASE}/curl.log"
  grep -Fq 'http://127.0.0.1:19090/proxies' "${TMPDIR_CASE}/curl.log"
  grep -Fq 'http://127.0.0.1:19090/version' "${TMPDIR_CASE}/curl.log"
}

test_status_reads_mode_from_controller() {
  setup_case
  run_manager render-config >/dev/null
  cat > "${TMPDIR_CASE}/controller-configs.json" <<'EOF'
{"mode":"global"}
EOF
  cat > "${TMPDIR_CASE}/controller-proxies.json" <<'EOF'
{"proxies":{"PROXY":{"type":"Selector","now":"AUTO","all":["DIRECT","AUTO"]},"AUTO":{"type":"URLTest","now":"manual-node","all":["manual-node"]}}}
EOF
  cat > "${TMPDIR_CASE}/controller-version.json" <<'EOF'
{"meta":true,"version":"Mihomo Meta v1.19.10"}
EOF
  output="$(run_manager status)"
  grep -q '服务状态: active' <<<"$output"
  grep -q '开机自启: enabled' <<<"$output"
  grep -q '当前模式: global' <<<"$output"
  grep -q '当前模式来源: Mihomo REST API' <<<"$output"
  grep -q '本地配置模式: rule' <<<"$output"
  grep -q '运行态策略组: PROXY=AUTO; AUTO=manual-node' <<<"$output"
  grep -q '控制面运行态: API 可达; 版本 Mihomo Meta v1.19.10' <<<"$output"
  grep -Fq 'http://127.0.0.1:19090/configs' "${TMPDIR_CASE}/curl.log"
  grep -Fq 'http://127.0.0.1:19090/proxies' "${TMPDIR_CASE}/curl.log"
  grep -Fq 'http://127.0.0.1:19090/version' "${TMPDIR_CASE}/curl.log"
}

test_status_recommends_router_ready_when_service_active() {
  setup_case
  python3 "${ROOT}/scripts/statectl.py" append-node "${TMPDIR_CASE}/state/nodes.json" 'vless://uuid@example.com:443?encryption=none&security=reality&sni=www.microsoft.com&fp=chrome&pbk=PUBLIC_KEY&sid=abcd&type=tcp#manual-node' manual-node 1 >/dev/null
  output="$(run_manager status)"
  grep -q '推荐下一步: 宿主机默认直连；局域网设备把网关和 DNS 指向 NAS 后即可走旁路由' <<<"$output"
}

test_healthcheck_uses_localhost_proxy_probe() {
  setup_case
  touch "${TMPDIR_CASE}/Country.mmdb"
  run_manager render-config >/dev/null
  run_manager healthcheck >/dev/null
  grep -Fq 'http://127.0.0.1:19090/ui/' "${TMPDIR_CASE}/curl.log"
  grep -Fq 'http://127.0.0.1:7890' "${TMPDIR_CASE}/curl.log"
  grep -Fq 'https://cp.cloudflare.com/generate_204' "${TMPDIR_CASE}/curl.log"
}

test_healthcheck_ignores_ss_pipefail_false_negative() {
  setup_case
  install_pipefail_ss_mock
  touch "${TMPDIR_CASE}/Country.mmdb"
  run_manager render-config >/dev/null
  output="$(run_manager healthcheck)"
  grep -q '健康检查通过' <<<"$output"
  ! grep -q 'not listening' <<<"$output"
}

test_menu_survives_failed_healthcheck() {
  setup_case
  run_manager render-config >/dev/null
  output="$(printf '7\n1\n0\n' | run_manager menu 2>&1)"
  grep -q 'geo: missing Country.mmdb' <<<"$output"
  [[ "$(grep -c 'Mihomo 管理器 v0.6.0' <<<"$output")" -ge 2 ]]
}

test_install_self_sync_writes_units_and_status() {
  setup_case
  run_manager install-self-sync 5 >/dev/null
  [[ -x "${TMPDIR_CASE}/install-root/mihomo" ]]
  [[ -L "${TMPDIR_CASE}/mihomo" ]]
  [[ ! -d "${TMPDIR_CASE}/install-root/.git" ]]
  [[ ! -d "${TMPDIR_CASE}/install-root/.codex" ]]
  grep -q '^MANAGER_SYNC_ENABLED="1"$' "${TMPDIR_CASE}/settings.env"
  grep -q '^MANAGER_SYNC_INTERVAL_MINUTES="5"$' "${TMPDIR_CASE}/settings.env"
  grep -Fq "ExecStart=${ROOT}/mihomo install-self" "${TMPDIR_CASE}/mihomo-manager-sync.service"
  grep -q '^OnUnitActiveSec=5min$' "${TMPDIR_CASE}/mihomo-manager-sync.timer"
  output="$(run_manager status)"
  grep -q "本机源码同步: 启用；每 5 分钟从 ${ROOT} 同步" <<<"$output"
}

test_disable_self_sync_removes_units() {
  setup_case
  run_manager install-self-sync 2 >/dev/null
  run_manager disable-self-sync >/dev/null
  grep -q '^MANAGER_SYNC_ENABLED="0"$' "${TMPDIR_CASE}/settings.env"
  [[ ! -f "${TMPDIR_CASE}/mihomo-manager-sync.service" ]]
  [[ ! -f "${TMPDIR_CASE}/mihomo-manager-sync.timer" ]]
}

test_install_geosite_downloads_official_asset() {
  setup_case
  run_manager install-geosite >/dev/null
  grep -Fq 'https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/geosite.dat' "${TMPDIR_CASE}/curl.log"
  [[ -f "${TMPDIR_CASE}/GeoSite.dat" ]]
}

test_install_webui_persists_external_ui_source() {
  setup_case
  touch "${TMPDIR_CASE}/unzip-ok"
  run_manager install-webui metacubexd https://github.com/MetaCubeX/metacubexd/archive/refs/heads/gh-pages.zip >/dev/null
  grep -q '^EXTERNAL_UI_NAME="metacubexd"$' "${TMPDIR_CASE}/settings.env"
  grep -q '^EXTERNAL_UI_URL="https://github.com/MetaCubeX/metacubexd/archive/refs/heads/gh-pages.zip"$' "${TMPDIR_CASE}/settings.env"
}

test_setup_bootstraps_empty_installation_even_when_webui_fails() {
  setup_case
  rm -f "${TMPDIR_CASE}/router.env" "${TMPDIR_CASE}/settings.env" "${TMPDIR_CASE}/Country.mmdb" "${TMPDIR_CASE}/GeoSite.dat"
  rm -rf "${TMPDIR_CASE}/state" "${TMPDIR_CASE}/ruleset" "${TMPDIR_CASE}/proxy_providers" "${TMPDIR_CASE}/ui"
  run_manager setup >/tmp/mh-setup-bootstrap.out 2>/tmp/mh-setup-bootstrap.err
  [[ -f "${TMPDIR_CASE}/Country.mmdb" ]]
  [[ -f "${TMPDIR_CASE}/GeoSite.dat" ]]
  [[ -f "${TMPDIR_CASE}/config.yaml" ]]
  [[ -f "${TMPDIR_CASE}/mihomo.service" ]]
  [[ -f "${TMPDIR_CASE}/state/nodes.json" ]]
  grep -q 'DOMAIN-SUFFIX,smzdm.com,DIRECT' "${TMPDIR_CASE}/ruleset/builtin.rules"
  grep -q 'DOMAIN-SUFFIX,smzdm.com,DIRECT' "${TMPDIR_CASE}/config.yaml"
  grep -Fq 'country.mmdb' "${TMPDIR_CASE}/curl.log"
  grep -Fq 'geosite.dat' "${TMPDIR_CASE}/curl.log"
  grep -q '核心旁路由链已继续' /tmp/mh-setup-bootstrap.out || grep -q '核心旁路由链已继续' /tmp/mh-setup-bootstrap.err
}

test_enable_start_after_cold_setup() {
  setup_case
  rm -f "${TMPDIR_CASE}/router.env" "${TMPDIR_CASE}/settings.env" "${TMPDIR_CASE}/Country.mmdb" "${TMPDIR_CASE}/GeoSite.dat"
  rm -rf "${TMPDIR_CASE}/state" "${TMPDIR_CASE}/ruleset" "${TMPDIR_CASE}/proxy_providers" "${TMPDIR_CASE}/ui"
  python3 "${ROOT}/scripts/statectl.py" append-node "${TMPDIR_CASE}/state/nodes.json" 'vless://uuid@example.com:443?encryption=none&security=reality&sni=www.microsoft.com&fp=chrome&pbk=PUBLIC_KEY&sid=abcd&type=tcp#cold-start-node' cold-start-node 1 >/dev/null
  run_manager setup >/dev/null 2>/tmp/mh-cold-setup.err
  run_manager enable-start >/dev/null
  grep -Fq 'enable --now mihomo' "${TMPDIR_CASE}/systemctl.log"
}

test_repair_restores_missing_assets() {
  setup_case
  run_manager render-config >/dev/null
  rm -f "${TMPDIR_CASE}/Country.mmdb" "${TMPDIR_CASE}/GeoSite.dat" "${TMPDIR_CASE}/mihomo.service"
  run_manager repair >/dev/null
  [[ -f "${TMPDIR_CASE}/Country.mmdb" ]]
  [[ -f "${TMPDIR_CASE}/GeoSite.dat" ]]
  [[ -f "${TMPDIR_CASE}/mihomo.service" ]]
}

test_update_subscriptions_refreshes_provider_cache() {
  setup_case
  run_manager add-subscription demo https://subscription.example/list.txt 1 >/dev/null
  run_manager update-subscriptions >/dev/null
  sub_id="$(python3 "${ROOT}/scripts/statectl.py" list-subscriptions "${TMPDIR_CASE}/state/subscriptions.json" | awk -F'\t' 'NR==1{print $2}')"
  [[ -s "${TMPDIR_CASE}/proxy_providers/subscriptions/${sub_id}.txt" ]]
  grep -q 'sub-vless' "${TMPDIR_CASE}/proxy_providers/subscriptions/${sub_id}.txt"
  grep -q '^proxies: \[\]$' "${TMPDIR_CASE}/proxy_providers/manual.txt"
  grep -q "./proxy_providers/subscriptions/${sub_id}.txt" "${TMPDIR_CASE}/config.yaml"
  grep -q 'subscription-' "${TMPDIR_CASE}/config.yaml"
  if python3 - "${TMPDIR_CASE}/state/nodes.json" <<'PY'
import json
import sys

with open(sys.argv[1], "r", encoding="utf-8") as f:
    data = json.load(f)
nodes = data.get("nodes", [])
raise SystemExit(0 if any((n.get("source") or {}).get("kind") == "subscription" for n in nodes) else 1)
PY
  then
    echo "subscription cache nodes should not be stored in nodes.json after update" >&2
    exit 1
  fi
}

test_rollback_config_restores_template() {
  setup_case
  run_manager render-config >/dev/null
  run_manager set-template nas-explicit-proxy-only >/dev/null
  grep -q 'TEMPLATE_NAME="nas-explicit-proxy-only"' "${TMPDIR_CASE}/router.env"
  run_manager rollback-config >/dev/null
  grep -q 'TEMPLATE_NAME="nas-single-lan-v4"' "${TMPDIR_CASE}/router.env"
}

main() {
  test_start_without_nodes_fails_before_systemctl_start
  test_configure_restart_enables_timer
  test_disable_alpha_update_disables_timer
  test_runtime_audit_outputs
  test_healthcheck_uses_localhost_proxy_probe
  test_healthcheck_ignores_ss_pipefail_false_negative
  test_menu_survives_failed_healthcheck
  test_install_self_sync_writes_units_and_status
  test_disable_self_sync_removes_units
  test_install_geosite_downloads_official_asset
  test_install_webui_persists_external_ui_source
  test_setup_bootstraps_empty_installation_even_when_webui_fails
  test_enable_start_after_cold_setup
  test_repair_restores_missing_assets
  test_update_subscriptions_refreshes_provider_cache
  test_rollback_config_restores_template
  echo "service-mock: ok"
}

main "$@"
