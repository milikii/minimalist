#!/usr/bin/env bash

set -euo pipefail

SCRIPT_VERSION="${SCRIPT_VERSION:-0.5.0}"

APP_ROOT="${APP_ROOT:-/usr/local/lib/mihomo-manager}"
STATECTL="${STATECTL:-${APP_ROOT}/scripts/statectl.py}"

MIHOMO_DIR="${MIHOMO_DIR:-/etc/mihomo}"
MIHOMO_BIN="${MIHOMO_BIN:-/usr/local/bin/mihomo-core}"
MIHOMO_USER="${MIHOMO_USER:-mihomo}"
MANAGER_BIN="${MANAGER_BIN:-/usr/local/bin/mihomo}"
COMPAT_MANAGER_BIN="${COMPAT_MANAGER_BIN:-/usr/local/bin/mihomo-sidecar.sh}"

ROUTER_ENV="${ROUTER_ENV:-${MIHOMO_DIR}/router.env}"
SETTINGS_ENV="${SETTINGS_ENV:-${MIHOMO_DIR}/settings.env}"
CONFIG_FILE="${CONFIG_FILE:-${MIHOMO_DIR}/config.yaml}"
RULES_DIR="${RULES_DIR:-${MIHOMO_DIR}/ruleset}"
PROVIDER_DIR="${PROVIDER_DIR:-${MIHOMO_DIR}/proxy_providers}"
UI_DIR="${UI_DIR:-${MIHOMO_DIR}/ui}"
COUNTRY_MMDB="${COUNTRY_MMDB:-${MIHOMO_DIR}/Country.mmdb}"
STATE_DIR="${STATE_DIR:-${MIHOMO_DIR}/state}"
NODES_STATE_FILE="${NODES_STATE_FILE:-${STATE_DIR}/nodes.json}"
RULES_STATE_FILE="${RULES_STATE_FILE:-${STATE_DIR}/rules.json}"
PROVIDER_FILE="${PROVIDER_FILE:-${PROVIDER_DIR}/manual.txt}"
RENDERED_RULES_FILE="${RENDERED_RULES_FILE:-${RULES_DIR}/custom.rules}"

ROUTER_SYSCTL="${ROUTER_SYSCTL:-/etc/sysctl.d/99-mihomo-router.conf}"
SYSTEMD_UNIT="${SYSTEMD_UNIT:-/etc/systemd/system/mihomo.service}"
RESTART_SERVICE_UNIT="${RESTART_SERVICE_UNIT:-/etc/systemd/system/mihomo-restart.service}"
RESTART_TIMER_UNIT="${RESTART_TIMER_UNIT:-/etc/systemd/system/mihomo-restart.timer}"
UPDATE_SERVICE_UNIT="${UPDATE_SERVICE_UNIT:-/etc/systemd/system/mihomo-alpha-update.service}"
UPDATE_TIMER_UNIT="${UPDATE_TIMER_UNIT:-/etc/systemd/system/mihomo-alpha-update.timer}"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

info() { echo -e "${BLUE}[i]${NC} $*"; }
ok() { echo -e "${GREEN}[+]${NC} $*"; }
warn() { echo -e "${YELLOW}[!]${NC} $*"; }
die() { echo -e "${RED}[x]${NC} $*" >&2; exit 1; }

have() { command -v "$1" >/dev/null 2>&1; }

user_exists() {
  id "$1" >/dev/null 2>&1
}

systemctl_cmd() {
  if [[ -n "${SYSTEMCTL_BIN:-}" ]]; then
    "$SYSTEMCTL_BIN" "$@"
  else
    systemctl "$@"
  fi
}

journalctl_cmd() {
  if [[ -n "${JOURNALCTL_BIN:-}" ]]; then
    "$JOURNALCTL_BIN" "$@"
  else
    journalctl "$@"
  fi
}

ss_cmd() {
  if [[ -n "${SS_BIN:-}" ]]; then
    "$SS_BIN" "$@"
  else
    ss "$@"
  fi
}

curl_cmd() {
  if [[ -n "${CURL_BIN:-}" ]]; then
    "$CURL_BIN" "$@"
  else
    curl "$@"
  fi
}

git_cmd() {
  if [[ -n "${GIT_BIN:-}" ]]; then
    "$GIT_BIN" "$@"
  else
    git "$@"
  fi
}

systemctl_show_value() {
  local unit="$1"
  local prop="$2"
  systemctl_cmd show "$unit" -p "$prop" --value 2>/dev/null || true
}

require_root() {
  [[ ${EUID} -eq 0 ]] || die "请用 root 运行: sudo mihomo"
}

require_statectl() {
  [[ -x "$STATECTL" ]] || die "未找到状态工具: $STATECTL"
}

ensure_state_files() {
  require_statectl
  python3 "$STATECTL" ensure-nodes-state "$NODES_STATE_FILE" "$PROVIDER_FILE" >/dev/null
  python3 "$STATECTL" ensure-rules-state "$RULES_STATE_FILE" "$RENDERED_RULES_FILE" >/dev/null
}

ipt() {
  iptables -w 5 "$@"
}

random_secret() {
  od -An -N16 -tx1 /dev/urandom | tr -d ' \n' | cut -c1-24
}

cidr_network() {
  local cidr="$1"
  [[ -n "$cidr" ]] || return 1
  python3 - "$cidr" <<'PY'
import ipaddress
import sys
print(ipaddress.ip_interface(sys.argv[1]).network)
PY
}

detect_default_iface() {
  ip -o route get 1.1.1.1 | awk '
    {
      for (i = 1; i <= NF; i++) {
        if ($i == "dev" && (i + 1) <= NF) {
          print $(i + 1)
          exit
        }
      }
    }
  '
}

detect_iface_cidr() {
  local iface="$1"
  ip -o -4 addr show dev "$iface" scope global | awk 'NR == 1 { print $4 }'
}

detect_iface_ip() {
  local iface="$1"
  detect_iface_cidr "$iface" | cut -d/ -f1
}

escape_env_value() {
  printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g'
}

upsert_env_var() {
  local file="$1"
  local key="$2"
  local value="$3"
  local escaped
  local tmp

  escaped="$(escape_env_value "$value")"
  tmp="$(mktemp)"

  if [[ -f "$file" ]]; then
    awk -v key="$key" '
      $0 ~ "^" key "=" { next }
      { print }
    ' "$file" >"$tmp"
  fi

  printf '%s="%s"\n' "$key" "$escaped" >>"$tmp"
  mv "$tmp" "$file"
}

ensure_settings() {
  mkdir -p "$MIHOMO_DIR"
  [[ -f "$SETTINGS_ENV" ]] || cat >"$SETTINGS_ENV" <<'EOF'
CONFIG_MODE="rule"
CORE_CHANNEL="alpha"
ALPHA_AUTO_UPDATE="0"
ALPHA_UPDATE_ONCALENDAR="daily"
RESTART_INTERVAL_HOURS="0"
RULES_AUTO_SYNC="1"
RULES_REPO_DIR="/root/mihomo-rules"
EOF
  chmod 640 "$SETTINGS_ENV"
}

ensure_router_env() {
  mkdir -p "$MIHOMO_DIR"
  if [[ ! -f "$ROUTER_ENV" ]]; then
    local lan_iface
    local lan_cidr
    lan_iface="$(detect_default_iface || true)"
    lan_cidr="$(detect_iface_cidr "${lan_iface:-}" || true)"
    lan_cidr="$(cidr_network "${lan_cidr:-}" 2>/dev/null || printf '%s' "${lan_cidr:-}")"
    cat >"$ROUTER_ENV" <<EOF
LAN_INTERFACES="${lan_iface:-bridge1}"
LAN_CIDRS="${lan_cidr:-192.168.2.0/24}"
PROXY_INGRESS_INTERFACES="${lan_iface:-bridge1}"
DNS_HIJACK_ENABLED="1"
DNS_HIJACK_INTERFACES="${lan_iface:-bridge1}"
PROXY_HOST_OUTPUT="0"
BYPASS_CONTAINER_NAMES="tr-bt TR1 Tr-music TR-plex"
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
    chmod 640 "$ROUTER_ENV"
  fi
}

load_settings() {
  ensure_settings
  # shellcheck disable=SC1090
  source "$SETTINGS_ENV"
  RULES_AUTO_SYNC="${RULES_AUTO_SYNC:-1}"
  RULES_REPO_DIR="${RULES_REPO_DIR:-/root/mihomo-rules}"
}

load_settings_readonly() {
  CONFIG_MODE="rule"
  CORE_CHANNEL="alpha"
  ALPHA_AUTO_UPDATE="0"
  ALPHA_UPDATE_ONCALENDAR="daily"
  RESTART_INTERVAL_HOURS="0"
  RULES_AUTO_SYNC="1"
  RULES_REPO_DIR="/root/mihomo-rules"
  if [[ -f "$SETTINGS_ENV" ]]; then
    # shellcheck disable=SC1090
    source "$SETTINGS_ENV"
  fi
}

load_router_env() {
  ensure_router_env
  # shellcheck disable=SC1090
  source "$ROUTER_ENV"

  MIXED_PORT="${MIXED_PORT:-7890}"
  TPROXY_PORT="${TPROXY_PORT:-7893}"
  DNS_PORT="${DNS_PORT:-1053}"
  CONTROLLER_PORT="${CONTROLLER_PORT:-19090}"
  ROUTE_MARK="${ROUTE_MARK:-0x2333}"
  ROUTE_MASK="${ROUTE_MASK:-0xffffffff}"
  ROUTE_TABLE="${ROUTE_TABLE:-233}"
  ROUTE_PRIORITY="${ROUTE_PRIORITY:-100}"
  PROXY_HOST_OUTPUT="${PROXY_HOST_OUTPUT:-0}"
  DNS_HIJACK_ENABLED="${DNS_HIJACK_ENABLED:-1}"
  LAN_INTERFACES="${LAN_INTERFACES:-}"
  LAN_CIDRS="${LAN_CIDRS:-}"
  PROXY_INGRESS_INTERFACES="${PROXY_INGRESS_INTERFACES:-$LAN_INTERFACES}"
  DNS_HIJACK_INTERFACES="${DNS_HIJACK_INTERFACES:-$LAN_INTERFACES}"
  BYPASS_CONTAINER_NAMES="${BYPASS_CONTAINER_NAMES:-}"
  BYPASS_SRC_CIDRS="${BYPASS_SRC_CIDRS:-}"
  BYPASS_DST_CIDRS="${BYPASS_DST_CIDRS:-}"
  BYPASS_UIDS="${BYPASS_UIDS:-}"

  ROUTE_MARK_DEC=$((ROUTE_MARK))
  read -r -a PROXY_INGRESS_IFACES_ARR <<< "${PROXY_INGRESS_INTERFACES}"
  read -r -a DNS_HIJACK_IFACES_ARR <<< "${DNS_HIJACK_INTERFACES}"
  read -r -a BYPASS_CONTAINER_NAMES_ARR <<< "${BYPASS_CONTAINER_NAMES}"
  read -r -a BYPASS_SRC_CIDRS_ARR <<< "${BYPASS_SRC_CIDRS}"
  read -r -a BYPASS_DST_CIDRS_ARR <<< "${BYPASS_DST_CIDRS}"
  read -r -a BYPASS_UIDS_ARR <<< "${BYPASS_UIDS}"

  RESERVED_DST_CIDRS_ARR=(
    "0.0.0.0/8"
    "10.0.0.0/8"
    "100.64.0.0/10"
    "127.0.0.0/8"
    "169.254.0.0/16"
    "172.16.0.0/12"
    "192.168.0.0/16"
    "224.0.0.0/4"
    "240.0.0.0/4"
    "255.255.255.255/32"
  )
}

load_router_env_readonly() {
  MIXED_PORT="7890"
  TPROXY_PORT="7893"
  DNS_PORT="1053"
  CONTROLLER_PORT="19090"
  ROUTE_MARK="0x2333"
  ROUTE_MASK="0xffffffff"
  ROUTE_TABLE="233"
  ROUTE_PRIORITY="100"
  PROXY_HOST_OUTPUT="0"
  DNS_HIJACK_ENABLED="1"
  LAN_INTERFACES=""
  LAN_CIDRS=""
  PROXY_INGRESS_INTERFACES=""
  DNS_HIJACK_INTERFACES=""
  BYPASS_CONTAINER_NAMES=""
  BYPASS_SRC_CIDRS=""
  BYPASS_DST_CIDRS=""
  BYPASS_UIDS=""
  if [[ -f "$ROUTER_ENV" ]]; then
    # shellcheck disable=SC1090
    source "$ROUTER_ENV"
  fi
}

ensure_layout() {
  mkdir -p "$MIHOMO_DIR" "$RULES_DIR" "$PROVIDER_DIR" "$UI_DIR" "$STATE_DIR"
  [[ -f "$PROVIDER_FILE" ]] || : >"$PROVIDER_FILE"
  [[ -f "$RENDERED_RULES_FILE" ]] || : >"$RENDERED_RULES_FILE"
  ensure_settings
  ensure_router_env
  ensure_state_files
}

node_enabled_count() {
  require_statectl
  python3 "$STATECTL" enabled-count "$NODES_STATE_FILE"
}

node_list_tsv() {
  require_statectl
  python3 "$STATECTL" list-nodes "$NODES_STATE_FILE"
}

node_enabled_names() {
  require_statectl
  python3 "$STATECTL" enabled-names "$NODES_STATE_FILE"
}

node_all_names() {
  require_statectl
  python3 "$STATECTL" all-names "$NODES_STATE_FILE"
}

readonly_node_counts() {
  if [[ -f "$NODES_STATE_FILE" ]]; then
    python3 - "$NODES_STATE_FILE" <<'PY'
import json
import sys

with open(sys.argv[1], "r", encoding="utf-8") as f:
    data = json.load(f)
nodes = data.get("nodes", [])
enabled = sum(1 for n in nodes if n.get("enabled"))
print(f"{enabled}\t{len(nodes)}")
PY
    return 0
  fi

  if [[ -f "$PROVIDER_FILE" ]]; then
    python3 - "$PROVIDER_FILE" <<'PY'
import sys

enabled = 0
total = 0
for raw in open(sys.argv[1], "r", encoding="utf-8"):
    line = raw.strip()
    if not line:
        continue
    if line.startswith("#DISABLED#") and "://" in line:
        total += 1
        continue
    if line.startswith("#"):
        continue
    if "://" in line:
        total += 1
        enabled += 1
print(f"{enabled}\t{total}")
PY
    return 0
  fi

  printf '0\t0\n'
}

ensure_enabled_nodes() {
  ensure_layout
  local enabled_count
  enabled_count="$(node_enabled_count)"
  [[ "$enabled_count" -gt 0 ]] || die "当前没有启用中的节点。先执行 'mihomo import-links' 导入节点，或用 'mihomo toggle-node' 启用已有节点，再启动/接管 mihomo"
}

host_output_proxy_enabled() {
  [[ "${PROXY_HOST_OUTPUT:-0}" == "1" ]]
}

print_host_output_proxy_warning() {
  host_output_proxy_enabled || return 0
  warn "宿主机透明接管已开启：宿主机 root 进程和系统守护进程也会被透明代理。"
  warn "这会影响 tailscaled、cloudflared、反向隧道、备份/同步任务等依赖稳定直连的服务。"
  warn "更安全的默认值是 PROXY_HOST_OUTPUT=0；宿主机应用如需代理，请显式使用 127.0.0.1:${MIXED_PORT}。"
}

unit_is_active() {
  local unit="$1"
  systemctl_cmd is-active --quiet "$unit" >/dev/null 2>&1
}

process_is_active() {
  local name="$1"
  if have pgrep; then
    pgrep -x "$name" >/dev/null 2>&1 && return 0
  fi
  ps -eo comm= 2>/dev/null | grep -Fxq "$name"
}

unit_or_process_is_active() {
  local unit="$1"
  local process_name="$2"
  unit_is_active "$unit" || process_is_active "$process_name"
}

guard_host_output_proxy_conflicts() {
  host_output_proxy_enabled || return 0
  local conflicts=()
  unit_or_process_is_active tailscaled.service tailscaled && conflicts+=("tailscaled")
  unit_or_process_is_active cloudflared.service cloudflared && conflicts+=("cloudflared")
  [[ ${#conflicts[@]} -eq 0 ]] && return 0
  warn "检测到以下宿主机关键服务正在运行: ${conflicts[*]}"
  warn "这些服务依赖宿主机稳定直连，不能和 PROXY_HOST_OUTPUT=1 混用。"
  die "修复: 保持 PROXY_HOST_OUTPUT=0；宿主机应用如需代理，请显式使用 127.0.0.1:${MIXED_PORT}；若你明确知道风险，先停掉 ${conflicts[*]} 再重试。"
}

service_is_active() {
  systemctl_cmd is-active --quiet mihomo >/dev/null 2>&1
}

service_is_enabled() {
  systemctl_cmd is-enabled mihomo >/dev/null 2>&1
}

restart_service_if_active() {
  if service_is_active; then
    ensure_layout
    if [[ "$(node_enabled_count)" -gt 0 ]]; then
      systemctl_cmd restart mihomo
      ok "已重启 mihomo"
    else
      systemctl_cmd stop mihomo
      warn "当前没有启用中的节点，已停止 mihomo 以避免空接管"
    fi
  fi
}

current_mode() {
  if [[ -f "$CONFIG_FILE" ]]; then
    awk '/^mode:/ { print $2; exit }' "$CONFIG_FILE"
  fi
  return 0
}
