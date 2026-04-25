#!/usr/bin/env bash

set -euo pipefail

SCRIPT_VERSION="${SCRIPT_VERSION:-0.6.0}"

APP_ROOT="${APP_ROOT:-/usr/local/lib/mihomo-manager}"
INSTALL_ROOT="${INSTALL_ROOT:-/usr/local/lib/mihomo-manager}"
STATECTL="${STATECTL:-${APP_ROOT}/scripts/statectl.py}"
RULEPRESETCTL="${RULEPRESETCTL:-${APP_ROOT}/scripts/rulepreset.py}"

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
ACL_STATE_FILE="${ACL_STATE_FILE:-${STATE_DIR}/acl.json}"
SUBSCRIPTIONS_STATE_FILE="${SUBSCRIPTIONS_STATE_FILE:-${STATE_DIR}/subscriptions.json}"
PROVIDER_FILE="${PROVIDER_FILE:-${PROVIDER_DIR}/manual.txt}"
RENDERED_RULES_FILE="${RENDERED_RULES_FILE:-${RULES_DIR}/custom.rules}"
ACL_RENDERED_RULES_FILE="${ACL_RENDERED_RULES_FILE:-${RULES_DIR}/acl.rules}"
RULESET_PRESET_RENDERED_FILE="${RULESET_PRESET_RENDERED_FILE:-${RULES_DIR}/builtin.rules}"
SNAPSHOT_DIR="${SNAPSHOT_DIR:-${STATE_DIR}/snapshots}"
RULE_REPO_ROOT="${RULE_REPO_ROOT:-${APP_ROOT}/rules-repo}"

ROUTER_SYSCTL="${ROUTER_SYSCTL:-/etc/sysctl.d/99-mihomo-router.conf}"
SYSTEMD_UNIT="${SYSTEMD_UNIT:-/etc/systemd/system/mihomo.service}"
RESTART_SERVICE_UNIT="${RESTART_SERVICE_UNIT:-/etc/systemd/system/mihomo-restart.service}"
RESTART_TIMER_UNIT="${RESTART_TIMER_UNIT:-/etc/systemd/system/mihomo-restart.timer}"
UPDATE_SERVICE_UNIT="${UPDATE_SERVICE_UNIT:-/etc/systemd/system/mihomo-alpha-update.service}"
UPDATE_TIMER_UNIT="${UPDATE_TIMER_UNIT:-/etc/systemd/system/mihomo-alpha-update.timer}"
MANAGER_SYNC_SERVICE_UNIT="${MANAGER_SYNC_SERVICE_UNIT:-/etc/systemd/system/mihomo-manager-sync.service}"
MANAGER_SYNC_TIMER_UNIT="${MANAGER_SYNC_TIMER_UNIT:-/etc/systemd/system/mihomo-manager-sync.timer}"

OVERRIDE_PROFILE_TEMPLATE="${PROFILE_TEMPLATE:-}"
OVERRIDE_RULESET_PRESET="${RULESET_PRESET:-}"

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

require_rulepresetctl() {
  [[ -x "$RULEPRESETCTL" ]] || die "未找到规则预设工具: $RULEPRESETCTL"
}

ensure_state_files() {
  require_statectl
  python3 "$STATECTL" ensure-nodes-state "$NODES_STATE_FILE" "$PROVIDER_FILE" >/dev/null
  python3 "$STATECTL" ensure-rules-state "$RULES_STATE_FILE" "$RENDERED_RULES_FILE" >/dev/null
  python3 "$STATECTL" ensure-rules-state "$ACL_STATE_FILE" >/dev/null
  python3 "$STATECTL" ensure-subscriptions-state "$SUBSCRIPTIONS_STATE_FILE" >/dev/null
}

iptables_cmd() {
  if [[ -n "${IPTABLES_BIN:-}" ]]; then
    "$IPTABLES_BIN" "$@"
  else
    iptables "$@"
  fi
}

ipt() {
  iptables_cmd -w 5 "$@"
}

controller_scope_summary() {
  CONTROLLER_HOST="${CONTROLLER_BIND_ADDRESS:-127.0.0.1}"
  CONTROLLER_SCOPE="仅宿主机"
  if [[ "$CONTROLLER_HOST" == "0.0.0.0" || "$CONTROLLER_HOST" == "*" ]]; then
    CONTROLLER_HOST="$(detect_iface_ip "${PROXY_INGRESS_INTERFACES%% *}" 2>/dev/null || echo 127.0.0.1)"
    CONTROLLER_SCOPE="局域网可访问(高风险)"
  fi
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

detect_iface_networks() {
  local ifaces="$1"
  local iface
  local cidr
  local networks=()

  read -r -a iface_arr <<< "$ifaces"
  for iface in "${iface_arr[@]}"; do
    [[ -n "$iface" ]] || continue
    cidr="$(detect_iface_cidr "$iface" || true)"
    [[ -n "$cidr" ]] || continue
    cidr="$(cidr_network "$cidr" 2>/dev/null || printf '%s' "$cidr")"
    [[ -n "$cidr" ]] && networks+=("$cidr")
  done

  [[ ${#networks[@]} -gt 0 ]] || return 0
  printf '%s
' "${networks[@]}" | sort -u | xargs
}

escape_env_value() {
  printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g'
}

unescape_env_value() {
  local input="$1"
  local output=""
  local idx=0
  local char
  local next

  while (( idx < ${#input} )); do
    char="${input:idx:1}"
    if [[ "$char" == "\\" ]] && (( idx + 1 < ${#input} )); then
      next="${input:idx+1:1}"
      if [[ "$next" == "\\" || "$next" == "\"" ]]; then
        output+="$next"
        idx=$((idx + 2))
        continue
      fi
    fi
    output+="$char"
    idx=$((idx + 1))
  done

  printf '%s' "$output"
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

read_env_var() {
  local file="$1"
  local key="$2"
  local fallback="${3:-}"
  local value
  if [[ ! -f "$file" ]]; then
    printf '%s\n' "$fallback"
    return 0
  fi
  value="$(
    awk -F= -v key="$key" -v fallback="$fallback" '
    $1 == key {
      value = substr($0, index($0, "=") + 1)
      print value
      found = 1
      exit
    }
    END {
      if (!found) {
        print fallback
      }
    }
  ' "$file"
  )"
  if [[ "$value" == '"'*'"' && ${#value} -ge 2 ]]; then
    value="${value:1:${#value}-2}"
  fi
  unescape_env_value "$value"
  printf '\n'
}

have_global_ipv6() {
  ip -6 route show default 2>/dev/null | grep -q .
}

default_profile_template() {
  printf '%s\n' "nas-single-lan-v4"
}

template_exists() {
  case "$1" in
    nas-single-lan-v4|nas-single-lan-dualstack|nas-multi-bridge|nas-explicit-proxy-only) return 0 ;;
    *) return 1 ;;
  esac
}

template_summary() {
  case "$1" in
    nas-single-lan-v4) printf '%s\n' "单 LAN IPv4 旁路由" ;;
    nas-single-lan-dualstack) printf '%s\n' "双栈模板占位（未实现真双栈旁路由）" ;;
    nas-multi-bridge) printf '%s\n' "多 bridge/VLAN 旁路由" ;;
    nas-explicit-proxy-only) printf '%s\n' "仅显式代理，不接管 LAN" ;;
    *) printf '%s\n' "未知模板" ;;
  esac
}

template_is_deprecated() {
  [[ "${1:-}" == "nas-single-lan-dualstack" ]]
}

default_rule_preset() {
  printf '%s\n' "default"
}

rule_preset_manifest_path() {
  case "$1" in
    default) printf '%s\n' "${RULE_REPO_ROOT}/default/manifest.yaml" ;;
    *) return 1 ;;
  esac
}

rule_preset_exists() {
  case "$1" in
    default) [[ -f "$(rule_preset_manifest_path "$1")" ]] ;;
    *) return 1 ;;
  esac
}

rule_preset_summary() {
  case "$1" in
    default) printf '%s\n' "项目内置默认模板：PT 直连，FCM 域名/IP 强制代理" ;;
    *) printf '%s\n' "未知规则预设" ;;
  esac
}

current_profile_template() {
  if [[ -f "$ROUTER_ENV" ]]; then
    read_env_var "$ROUTER_ENV" "TEMPLATE_NAME" "$(read_env_var "$SETTINGS_ENV" "PROFILE_TEMPLATE" "$(default_profile_template)")"
  else
    read_env_var "$SETTINGS_ENV" "PROFILE_TEMPLATE" "$(default_profile_template)"
  fi
}

write_router_env_from_template() {
  local template_name="$1"
  template_exists "$template_name" || die "未知模板: ${template_name}"

  local existing_mixed_port="7890"
  local existing_tproxy_port="7893"
  local existing_dns_port="1053"
  local existing_controller_port="19090"
  local existing_controller_bind="127.0.0.1"
  local existing_controller_cors_origins=""
  local existing_controller_cors_private="0"
  local existing_lan_disallowed=""
  local existing_proxy_auth=""
  local existing_skip_auth=""
  local existing_route_mark="0x2333"
  local existing_route_mask="0xffffffff"
  local existing_route_table="233"
  local existing_route_priority="100"
  local existing_host_output="0"
  local existing_bypass_src=""
  local existing_bypass_dst=""
  local existing_bypass_uids=""

  if [[ -f "$ROUTER_ENV" ]]; then
    existing_mixed_port="$(read_env_var "$ROUTER_ENV" "MIXED_PORT" "$existing_mixed_port")"
    existing_tproxy_port="$(read_env_var "$ROUTER_ENV" "TPROXY_PORT" "$existing_tproxy_port")"
    existing_dns_port="$(read_env_var "$ROUTER_ENV" "DNS_PORT" "$existing_dns_port")"
    existing_controller_port="$(read_env_var "$ROUTER_ENV" "CONTROLLER_PORT" "$existing_controller_port")"
    existing_controller_bind="$(read_env_var "$ROUTER_ENV" "CONTROLLER_BIND_ADDRESS" "$existing_controller_bind")"
    existing_controller_cors_origins="$(read_env_var "$ROUTER_ENV" "CONTROLLER_CORS_ALLOW_ORIGINS" "$existing_controller_cors_origins")"
    existing_controller_cors_private="$(read_env_var "$ROUTER_ENV" "CONTROLLER_CORS_ALLOW_PRIVATE_NETWORK" "$existing_controller_cors_private")"
    existing_lan_disallowed="$(read_env_var "$ROUTER_ENV" "LAN_DISALLOWED_CIDRS" "$existing_lan_disallowed")"
    existing_proxy_auth="$(read_env_var "$ROUTER_ENV" "PROXY_AUTH_CREDENTIALS" "$existing_proxy_auth")"
    existing_skip_auth="$(read_env_var "$ROUTER_ENV" "SKIP_AUTH_PREFIXES" "$existing_skip_auth")"
    existing_route_mark="$(read_env_var "$ROUTER_ENV" "ROUTE_MARK" "$existing_route_mark")"
    existing_route_mask="$(read_env_var "$ROUTER_ENV" "ROUTE_MASK" "$existing_route_mask")"
    existing_route_table="$(read_env_var "$ROUTER_ENV" "ROUTE_TABLE" "$existing_route_table")"
    existing_route_priority="$(read_env_var "$ROUTER_ENV" "ROUTE_PRIORITY" "$existing_route_priority")"
    existing_host_output="$(read_env_var "$ROUTER_ENV" "PROXY_HOST_OUTPUT" "$existing_host_output")"
    existing_bypass_src="$(read_env_var "$ROUTER_ENV" "BYPASS_SRC_CIDRS" "$existing_bypass_src")"
    existing_bypass_dst="$(read_env_var "$ROUTER_ENV" "BYPASS_DST_CIDRS" "$existing_bypass_dst")"
    existing_bypass_uids="$(read_env_var "$ROUTER_ENV" "BYPASS_UIDS" "$existing_bypass_uids")"
  fi

  local lan_iface
  local lan_cidr
  local enable_ipv6="0"
  local dns_hijack_enabled="1"
  local proxy_ingress_ifaces
  local dns_hijack_ifaces
  local bypass_containers=""

  lan_iface="$(detect_default_iface || true)"
  lan_cidr="$(detect_iface_cidr "${lan_iface:-}" || true)"
  lan_cidr="$(cidr_network "${lan_cidr:-}" 2>/dev/null || printf '%s' "${lan_cidr:-}")"
  proxy_ingress_ifaces="${lan_iface:-bridge1}"
  dns_hijack_ifaces="${lan_iface:-bridge1}"

  case "$template_name" in
    nas-single-lan-v4)
      enable_ipv6="0"
      ;;
    nas-single-lan-dualstack)
      enable_ipv6="1"
      ;;
    nas-multi-bridge)
      if have_global_ipv6; then
        enable_ipv6="1"
      fi
      ;;
    nas-explicit-proxy-only)
      if have_global_ipv6; then
        enable_ipv6="1"
      fi
      dns_hijack_enabled="0"
      proxy_ingress_ifaces=""
      dns_hijack_ifaces=""
      ;;
  esac

  mkdir -p "$MIHOMO_DIR"
  cat >"$ROUTER_ENV" <<EOF
TEMPLATE_NAME="${template_name}"
ENABLE_IPV6="${enable_ipv6}"
LAN_INTERFACES="${lan_iface:-bridge1}"
LAN_CIDRS="${lan_cidr:-192.168.2.0/24}"
LAN_DISALLOWED_CIDRS="${existing_lan_disallowed}"
PROXY_INGRESS_INTERFACES="${proxy_ingress_ifaces}"
DNS_HIJACK_ENABLED="${dns_hijack_enabled}"
DNS_HIJACK_INTERFACES="${dns_hijack_ifaces}"
PROXY_AUTH_CREDENTIALS="${existing_proxy_auth}"
SKIP_AUTH_PREFIXES="${existing_skip_auth}"
PROXY_HOST_OUTPUT="${existing_host_output}"
BYPASS_CONTAINER_NAMES="${bypass_containers}"
BYPASS_SRC_CIDRS="${existing_bypass_src}"
BYPASS_DST_CIDRS="${existing_bypass_dst}"
BYPASS_UIDS="${existing_bypass_uids}"
MIXED_PORT="${existing_mixed_port}"
TPROXY_PORT="${existing_tproxy_port}"
DNS_PORT="${existing_dns_port}"
CONTROLLER_PORT="${existing_controller_port}"
CONTROLLER_BIND_ADDRESS="${existing_controller_bind}"
CONTROLLER_CORS_ALLOW_ORIGINS="${existing_controller_cors_origins}"
CONTROLLER_CORS_ALLOW_PRIVATE_NETWORK="${existing_controller_cors_private}"
ROUTE_MARK="${existing_route_mark}"
ROUTE_MASK="${existing_route_mask}"
ROUTE_TABLE="${existing_route_table}"
ROUTE_PRIORITY="${existing_route_priority}"
EOF
  chmod 640 "$ROUTER_ENV"
}

ensure_settings() {
  mkdir -p "$MIHOMO_DIR"
  [[ -f "$SETTINGS_ENV" ]] || cat >"$SETTINGS_ENV" <<'EOF'
CONFIG_MODE="rule"
CORE_CHANNEL="alpha"
ALPHA_AUTO_UPDATE="0"
ALPHA_UPDATE_ONCALENDAR="daily"
RESTART_INTERVAL_HOURS="0"
RULESET_PRESET="default"
EXTERNAL_UI_NAME=""
EXTERNAL_UI_URL=""
MANAGER_SYNC_ENABLED="0"
MANAGER_SYNC_INTERVAL_MINUTES="1"
MANAGER_SYNC_SOURCE=""
EOF
  local template_name
  local rule_preset_name
  template_name="$(read_env_var "$SETTINGS_ENV" "PROFILE_TEMPLATE" "")"
  if ! template_exists "$template_name"; then
    upsert_env_var "$SETTINGS_ENV" "PROFILE_TEMPLATE" "$(default_profile_template)"
  fi
  rule_preset_name="$(read_env_var "$SETTINGS_ENV" "RULESET_PRESET" "")"
  if ! rule_preset_exists "$rule_preset_name"; then
    upsert_env_var "$SETTINGS_ENV" "RULESET_PRESET" "$(default_rule_preset)"
  fi
  chmod 640 "$SETTINGS_ENV"
}

ensure_router_env() {
  mkdir -p "$MIHOMO_DIR"
  if [[ ! -f "$ROUTER_ENV" ]]; then
    write_router_env_from_template "$(read_env_var "$SETTINGS_ENV" "PROFILE_TEMPLATE" "$(default_profile_template)")"
  fi
}

load_settings() {
  ensure_settings
  CONFIG_MODE="$(read_env_var "$SETTINGS_ENV" "CONFIG_MODE" "rule")"
  CORE_CHANNEL="$(read_env_var "$SETTINGS_ENV" "CORE_CHANNEL" "alpha")"
  ALPHA_AUTO_UPDATE="$(read_env_var "$SETTINGS_ENV" "ALPHA_AUTO_UPDATE" "0")"
  ALPHA_UPDATE_ONCALENDAR="$(read_env_var "$SETTINGS_ENV" "ALPHA_UPDATE_ONCALENDAR" "daily")"
  RESTART_INTERVAL_HOURS="$(read_env_var "$SETTINGS_ENV" "RESTART_INTERVAL_HOURS" "0")"
  PROFILE_TEMPLATE="${OVERRIDE_PROFILE_TEMPLATE:-$(read_env_var "$SETTINGS_ENV" "PROFILE_TEMPLATE" "$(default_profile_template)")}"
  RULESET_PRESET="${OVERRIDE_RULESET_PRESET:-$(read_env_var "$SETTINGS_ENV" "RULESET_PRESET" "$(default_rule_preset)")}"
  EXTERNAL_UI_NAME="$(read_env_var "$SETTINGS_ENV" "EXTERNAL_UI_NAME" "")"
  EXTERNAL_UI_URL="$(read_env_var "$SETTINGS_ENV" "EXTERNAL_UI_URL" "")"
  MANAGER_SYNC_ENABLED="$(read_env_var "$SETTINGS_ENV" "MANAGER_SYNC_ENABLED" "0")"
  MANAGER_SYNC_INTERVAL_MINUTES="$(read_env_var "$SETTINGS_ENV" "MANAGER_SYNC_INTERVAL_MINUTES" "1")"
  MANAGER_SYNC_SOURCE="$(read_env_var "$SETTINGS_ENV" "MANAGER_SYNC_SOURCE" "")"
}

load_settings_readonly() {
  CONFIG_MODE="$(read_env_var "$SETTINGS_ENV" "CONFIG_MODE" "rule")"
  CORE_CHANNEL="$(read_env_var "$SETTINGS_ENV" "CORE_CHANNEL" "alpha")"
  ALPHA_AUTO_UPDATE="$(read_env_var "$SETTINGS_ENV" "ALPHA_AUTO_UPDATE" "0")"
  ALPHA_UPDATE_ONCALENDAR="$(read_env_var "$SETTINGS_ENV" "ALPHA_UPDATE_ONCALENDAR" "daily")"
  RESTART_INTERVAL_HOURS="$(read_env_var "$SETTINGS_ENV" "RESTART_INTERVAL_HOURS" "0")"
  PROFILE_TEMPLATE="${OVERRIDE_PROFILE_TEMPLATE:-$(read_env_var "$SETTINGS_ENV" "PROFILE_TEMPLATE" "$(default_profile_template)")}"
  RULESET_PRESET="${OVERRIDE_RULESET_PRESET:-$(read_env_var "$SETTINGS_ENV" "RULESET_PRESET" "$(default_rule_preset)")}"
  EXTERNAL_UI_NAME="$(read_env_var "$SETTINGS_ENV" "EXTERNAL_UI_NAME" "")"
  EXTERNAL_UI_URL="$(read_env_var "$SETTINGS_ENV" "EXTERNAL_UI_URL" "")"
  MANAGER_SYNC_ENABLED="$(read_env_var "$SETTINGS_ENV" "MANAGER_SYNC_ENABLED" "0")"
  MANAGER_SYNC_INTERVAL_MINUTES="$(read_env_var "$SETTINGS_ENV" "MANAGER_SYNC_INTERVAL_MINUTES" "1")"
  MANAGER_SYNC_SOURCE="$(read_env_var "$SETTINGS_ENV" "MANAGER_SYNC_SOURCE" "")"
}

load_router_env() {
  ensure_router_env
  MIXED_PORT="$(read_env_var "$ROUTER_ENV" "MIXED_PORT" "7890")"
  TPROXY_PORT="$(read_env_var "$ROUTER_ENV" "TPROXY_PORT" "7893")"
  DNS_PORT="$(read_env_var "$ROUTER_ENV" "DNS_PORT" "1053")"
  CONTROLLER_PORT="$(read_env_var "$ROUTER_ENV" "CONTROLLER_PORT" "19090")"
  CONTROLLER_BIND_ADDRESS="$(read_env_var "$ROUTER_ENV" "CONTROLLER_BIND_ADDRESS" "127.0.0.1")"
  CONTROLLER_CORS_ALLOW_ORIGINS="$(read_env_var "$ROUTER_ENV" "CONTROLLER_CORS_ALLOW_ORIGINS" "")"
  CONTROLLER_CORS_ALLOW_PRIVATE_NETWORK="$(read_env_var "$ROUTER_ENV" "CONTROLLER_CORS_ALLOW_PRIVATE_NETWORK" "0")"
  TEMPLATE_NAME="$(read_env_var "$ROUTER_ENV" "TEMPLATE_NAME" "$(current_profile_template)")"
  ENABLE_IPV6="$(read_env_var "$ROUTER_ENV" "ENABLE_IPV6" "0")"
  ROUTE_MARK="$(read_env_var "$ROUTER_ENV" "ROUTE_MARK" "0x2333")"
  ROUTE_MASK="$(read_env_var "$ROUTER_ENV" "ROUTE_MASK" "0xffffffff")"
  ROUTE_TABLE="$(read_env_var "$ROUTER_ENV" "ROUTE_TABLE" "233")"
  ROUTE_PRIORITY="$(read_env_var "$ROUTER_ENV" "ROUTE_PRIORITY" "100")"
  PROXY_HOST_OUTPUT="$(read_env_var "$ROUTER_ENV" "PROXY_HOST_OUTPUT" "0")"
  DNS_HIJACK_ENABLED="$(read_env_var "$ROUTER_ENV" "DNS_HIJACK_ENABLED" "1")"
  LAN_INTERFACES="$(read_env_var "$ROUTER_ENV" "LAN_INTERFACES" "")"
  LAN_CIDRS="$(read_env_var "$ROUTER_ENV" "LAN_CIDRS" "")"
  LAN_DISALLOWED_CIDRS="$(read_env_var "$ROUTER_ENV" "LAN_DISALLOWED_CIDRS" "")"
  PROXY_INGRESS_INTERFACES="$(read_env_var "$ROUTER_ENV" "PROXY_INGRESS_INTERFACES" "$LAN_INTERFACES")"
  DNS_HIJACK_INTERFACES="$(read_env_var "$ROUTER_ENV" "DNS_HIJACK_INTERFACES" "$LAN_INTERFACES")"
  PROXY_AUTH_CREDENTIALS="$(read_env_var "$ROUTER_ENV" "PROXY_AUTH_CREDENTIALS" "")"
  SKIP_AUTH_PREFIXES="$(read_env_var "$ROUTER_ENV" "SKIP_AUTH_PREFIXES" "")"
  BYPASS_CONTAINER_NAMES="$(read_env_var "$ROUTER_ENV" "BYPASS_CONTAINER_NAMES" "")"
  BYPASS_SRC_CIDRS="$(read_env_var "$ROUTER_ENV" "BYPASS_SRC_CIDRS" "")"
  BYPASS_DST_CIDRS="$(read_env_var "$ROUTER_ENV" "BYPASS_DST_CIDRS" "")"
  BYPASS_UIDS="$(read_env_var "$ROUTER_ENV" "BYPASS_UIDS" "")"

  ROUTE_MARK_DEC=$((ROUTE_MARK))
  read -r -a PROXY_INGRESS_IFACES_ARR <<< "${PROXY_INGRESS_INTERFACES}"
  read -r -a DNS_HIJACK_IFACES_ARR <<< "${DNS_HIJACK_INTERFACES}"
  read -r -a CONTROLLER_CORS_ALLOW_ORIGINS_ARR <<< "${CONTROLLER_CORS_ALLOW_ORIGINS}"
  read -r -a LAN_DISALLOWED_CIDRS_ARR <<< "${LAN_DISALLOWED_CIDRS}"
  read -r -a PROXY_AUTH_CREDENTIALS_ARR <<< "${PROXY_AUTH_CREDENTIALS}"
  read -r -a SKIP_AUTH_PREFIXES_ARR <<< "${SKIP_AUTH_PREFIXES}"
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
  MIXED_PORT="$(read_env_var "$ROUTER_ENV" "MIXED_PORT" "7890")"
  TPROXY_PORT="$(read_env_var "$ROUTER_ENV" "TPROXY_PORT" "7893")"
  DNS_PORT="$(read_env_var "$ROUTER_ENV" "DNS_PORT" "1053")"
  CONTROLLER_PORT="$(read_env_var "$ROUTER_ENV" "CONTROLLER_PORT" "19090")"
  CONTROLLER_BIND_ADDRESS="$(read_env_var "$ROUTER_ENV" "CONTROLLER_BIND_ADDRESS" "127.0.0.1")"
  CONTROLLER_CORS_ALLOW_ORIGINS="$(read_env_var "$ROUTER_ENV" "CONTROLLER_CORS_ALLOW_ORIGINS" "")"
  CONTROLLER_CORS_ALLOW_PRIVATE_NETWORK="$(read_env_var "$ROUTER_ENV" "CONTROLLER_CORS_ALLOW_PRIVATE_NETWORK" "0")"
  TEMPLATE_NAME="$(read_env_var "$ROUTER_ENV" "TEMPLATE_NAME" "$(current_profile_template)")"
  ENABLE_IPV6="$(read_env_var "$ROUTER_ENV" "ENABLE_IPV6" "0")"
  ROUTE_MARK="$(read_env_var "$ROUTER_ENV" "ROUTE_MARK" "0x2333")"
  ROUTE_MASK="$(read_env_var "$ROUTER_ENV" "ROUTE_MASK" "0xffffffff")"
  ROUTE_TABLE="$(read_env_var "$ROUTER_ENV" "ROUTE_TABLE" "233")"
  ROUTE_PRIORITY="$(read_env_var "$ROUTER_ENV" "ROUTE_PRIORITY" "100")"
  PROXY_HOST_OUTPUT="$(read_env_var "$ROUTER_ENV" "PROXY_HOST_OUTPUT" "0")"
  DNS_HIJACK_ENABLED="$(read_env_var "$ROUTER_ENV" "DNS_HIJACK_ENABLED" "1")"
  LAN_INTERFACES="$(read_env_var "$ROUTER_ENV" "LAN_INTERFACES" "")"
  LAN_CIDRS="$(read_env_var "$ROUTER_ENV" "LAN_CIDRS" "")"
  LAN_DISALLOWED_CIDRS="$(read_env_var "$ROUTER_ENV" "LAN_DISALLOWED_CIDRS" "")"
  PROXY_INGRESS_INTERFACES="$(read_env_var "$ROUTER_ENV" "PROXY_INGRESS_INTERFACES" "$LAN_INTERFACES")"
  DNS_HIJACK_INTERFACES="$(read_env_var "$ROUTER_ENV" "DNS_HIJACK_INTERFACES" "$LAN_INTERFACES")"
  PROXY_AUTH_CREDENTIALS="$(read_env_var "$ROUTER_ENV" "PROXY_AUTH_CREDENTIALS" "")"
  SKIP_AUTH_PREFIXES="$(read_env_var "$ROUTER_ENV" "SKIP_AUTH_PREFIXES" "")"
  BYPASS_CONTAINER_NAMES="$(read_env_var "$ROUTER_ENV" "BYPASS_CONTAINER_NAMES" "")"
  BYPASS_SRC_CIDRS="$(read_env_var "$ROUTER_ENV" "BYPASS_SRC_CIDRS" "")"
  BYPASS_DST_CIDRS="$(read_env_var "$ROUTER_ENV" "BYPASS_DST_CIDRS" "")"
  BYPASS_UIDS="$(read_env_var "$ROUTER_ENV" "BYPASS_UIDS" "")"
  read -r -a CONTROLLER_CORS_ALLOW_ORIGINS_ARR <<< "${CONTROLLER_CORS_ALLOW_ORIGINS}"
  read -r -a PROXY_AUTH_CREDENTIALS_ARR <<< "${PROXY_AUTH_CREDENTIALS}"
  read -r -a SKIP_AUTH_PREFIXES_ARR <<< "${SKIP_AUTH_PREFIXES}"
}

ensure_layout() {
  mkdir -p "$MIHOMO_DIR" "$RULES_DIR" "$PROVIDER_DIR" "$UI_DIR" "$STATE_DIR" "$SNAPSHOT_DIR"
  [[ -f "$PROVIDER_FILE" ]] || : >"$PROVIDER_FILE"
  [[ -f "$RENDERED_RULES_FILE" ]] || : >"$RENDERED_RULES_FILE"
  [[ -f "$ACL_RENDERED_RULES_FILE" ]] || : >"$ACL_RENDERED_RULES_FILE"
  [[ -f "$RULESET_PRESET_RENDERED_FILE" ]] || : >"$RULESET_PRESET_RENDERED_FILE"
  ensure_settings
  ensure_router_env
  ensure_state_files
}

copy_file_if_exists() {
  local src="$1"
  local dst="$2"
  [[ -f "$src" ]] || return 0
  install -D -m 0640 "$src" "$dst"
}

snapshot_current_state() {
  ensure_layout
  local label="${1:-manual}"
  local stamp
  local target
  stamp="$(date -u +%Y%m%dT%H%M%SZ)"
  target="${SNAPSHOT_DIR}/${stamp}-${label// /-}"
  mkdir -p "$target"
  copy_file_if_exists "$SETTINGS_ENV" "$target/settings.env"
  copy_file_if_exists "$ROUTER_ENV" "$target/router.env"
  copy_file_if_exists "$CONFIG_FILE" "$target/config.yaml"
  copy_file_if_exists "$NODES_STATE_FILE" "$target/nodes.json"
  copy_file_if_exists "$RULES_STATE_FILE" "$target/rules.json"
  copy_file_if_exists "$ACL_STATE_FILE" "$target/acl.json"
  copy_file_if_exists "$SUBSCRIPTIONS_STATE_FILE" "$target/subscriptions.json"
  copy_file_if_exists "$PROVIDER_FILE" "$target/manual.txt"
  copy_file_if_exists "$RENDERED_RULES_FILE" "$target/custom.rules"
  copy_file_if_exists "$ACL_RENDERED_RULES_FILE" "$target/acl.rules"
  if [[ -f "$SYSTEMD_UNIT" ]]; then
    copy_file_if_exists "$SYSTEMD_UNIT" "$target/mihomo.service"
  fi
  rm -rf "${SNAPSHOT_DIR}/latest"
  cp -a "$target" "${SNAPSHOT_DIR}/latest"
  printf '%s\n' "$target"
}

restore_latest_snapshot() {
  local latest="${SNAPSHOT_DIR}/latest"
  [[ -d "$latest" ]] || die "未找到可回滚快照: ${latest}"
  ensure_layout
  copy_file_if_exists "$latest/settings.env" "$SETTINGS_ENV"
  copy_file_if_exists "$latest/router.env" "$ROUTER_ENV"
  copy_file_if_exists "$latest/config.yaml" "$CONFIG_FILE"
  copy_file_if_exists "$latest/nodes.json" "$NODES_STATE_FILE"
  copy_file_if_exists "$latest/rules.json" "$RULES_STATE_FILE"
  copy_file_if_exists "$latest/acl.json" "$ACL_STATE_FILE"
  copy_file_if_exists "$latest/subscriptions.json" "$SUBSCRIPTIONS_STATE_FILE"
  copy_file_if_exists "$latest/manual.txt" "$PROVIDER_FILE"
  copy_file_if_exists "$latest/custom.rules" "$RENDERED_RULES_FILE"
  copy_file_if_exists "$latest/acl.rules" "$ACL_RENDERED_RULES_FILE"
  if [[ -f "$latest/mihomo.service" ]]; then
    copy_file_if_exists "$latest/mihomo.service" "$SYSTEMD_UNIT"
  fi
}

node_enabled_count() {
  require_statectl
  python3 "$STATECTL" enabled-count "$NODES_STATE_FILE"
}

node_list_tsv() {
  require_statectl
  python3 "$STATECTL" list-nodes "$NODES_STATE_FILE" --exclude-source-kind subscription
}

node_enabled_names() {
  require_statectl
  python3 "$STATECTL" enabled-names "$NODES_STATE_FILE" --exclude-source-kind subscription
}

node_all_names() {
  require_statectl
  python3 "$STATECTL" all-names "$NODES_STATE_FILE" --exclude-source-kind subscription
}

acl_list_tsv() {
  require_statectl
  python3 "$STATECTL" list-rules "$ACL_STATE_FILE"
}

manual_node_counts() {
  local enabled=0
  local total=0
  while IFS=$'\t' read -r _ enabled_flag _ _ _ _ _ _ source; do
    [[ "$source" == subscription:* ]] && continue
    total=$((total + 1))
    [[ "$enabled_flag" == "1" ]] && enabled=$((enabled + 1))
  done < <(node_list_tsv || true)
  printf '%s\t%s\n' "$enabled" "$total"
}

subscription_counts() {
  local enabled=0
  local total=0
  while IFS=$'\t' read -r _ _ _ _ enabled_flag _ _ _ _; do
    total=$((total + 1))
    [[ "$enabled_flag" == "1" ]] && enabled=$((enabled + 1))
  done < <(subscription_list_tsv || true)
  printf '%s\t%s\n' "$enabled" "$total"
}

subscription_provider_counts() {
  local enabled=0
  local total=0
  local ready=0
  local sub_id
  local enabled_flag
  while IFS=$'\t' read -r _ sub_id _ _ enabled_flag _ _ _; do
    total=$((total + 1))
    [[ "$enabled_flag" == "1" ]] && enabled=$((enabled + 1))
    [[ -s "$(subscription_provider_file "$sub_id")" ]] && ready=$((ready + 1))
  done < <(subscription_list_tsv || true)
  printf '%s\t%s\t%s\n' "$enabled" "$total" "$ready"
}

subscription_list_tsv() {
  require_statectl
  python3 "$STATECTL" list-subscriptions "$SUBSCRIPTIONS_STATE_FILE"
}

subscription_provider_dir() {
  printf '%s\n' "${PROVIDER_DIR}/subscriptions"
}

subscription_provider_file() {
  local subscription_id="$1"
  printf '%s/%s.txt\n' "$(subscription_provider_dir)" "$subscription_id"
}

subscription_provider_relpath() {
  local subscription_id="$1"
  printf './proxy_providers/subscriptions/%s.txt\n' "$subscription_id"
}

subscription_provider_name() {
  local subscription_id="$1"
  local short_id="${subscription_id%%-*}"
  [[ -n "$short_id" ]] || short_id="$subscription_id"
  printf 'subscription-%s\n' "$short_id"
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
  current_mode_with_source | awk -F $'\t' 'NR == 1 { print $1 }'
}

configured_mode() {
  if [[ -f "$CONFIG_FILE" ]]; then
    awk '/^mode:/ { print $2; exit }' "$CONFIG_FILE"
  fi
  return 0
}

controller_api_host() {
  case "${CONTROLLER_BIND_ADDRESS:-127.0.0.1}" in
    ""|0.0.0.0|"*") printf '%s\n' "127.0.0.1" ;;
    *) printf '%s\n' "${CONTROLLER_BIND_ADDRESS}" ;;
  esac
}

controller_api_url() {
  local path="${1:-/}"
  printf 'http://%s:%s%s\n' "$(controller_api_host)" "${CONTROLLER_PORT:-19090}" "$path"
}

controller_secret() {
  if [[ -f "$CONFIG_FILE" ]]; then
    awk -F '"' '/^secret:/ { print $2; exit }' "$CONFIG_FILE"
  fi
  return 0
}

controller_api_get() {
  local path="$1"
  local url secret

  url="$(controller_api_url "$path")"
  secret="$(controller_secret)"
  if [[ -n "$secret" ]]; then
    curl_cmd --noproxy '*' -fsS --connect-timeout 1 --max-time 1 -H "Authorization: Bearer ${secret}" "$url"
    return 0
  fi
  curl_cmd --noproxy '*' -fsS --connect-timeout 1 --max-time 1 "$url"
}

runtime_mode() {
  local json
  json="$(controller_api_get "/configs" 2>/dev/null)" || return 1
  python3 -c 'import json, sys
try:
    data = json.load(sys.stdin)
except Exception:
    raise SystemExit(1)
mode = data.get("mode")
if not isinstance(mode, str) or not mode:
    raise SystemExit(1)
print(mode)
' <<<"$json"
}

current_mode_with_source() {
  local mode_value

  if mode_value="$(runtime_mode 2>/dev/null)"; then
    printf '%s\t%s\n' "$mode_value" "Mihomo REST API"
    return 0
  fi

  mode_value="$(configured_mode || true)"
  [[ -n "$mode_value" ]] || mode_value="rule"
  printf '%s\t%s\n' "$mode_value" "本地配置回退"
}

runtime_policy_group_summary() {
  local json
  json="$(controller_api_get "/proxies" 2>/dev/null)" || return 1
  python3 -c 'import json, sys
try:
    data = json.load(sys.stdin)
except Exception:
    raise SystemExit(1)
proxies = data.get("proxies")
if not isinstance(proxies, dict):
    raise SystemExit(1)
summary = []
for name, item in proxies.items():
    if not isinstance(item, dict):
        continue
    all_values = item.get("all")
    now = item.get("now")
    if not isinstance(all_values, list) or not all_values:
        continue
    if not isinstance(now, str) or not now:
        continue
    summary.append(f"{name}={now}")
if not summary:
    raise SystemExit(1)
print("; ".join(summary))
' <<<"$json"
}

controller_runtime_summary() {
  local raw
  raw="$(controller_api_get "/version" 2>/dev/null)" || return 1
  python3 -c 'import json, sys
raw = sys.stdin.read().strip()
if not raw:
    raise SystemExit(1)
try:
    data = json.loads(raw)
except Exception:
    print(f"API 可达; 版本 {raw}")
    raise SystemExit(0)
version = data.get("version")
if not isinstance(version, str) or not version:
    raise SystemExit(1)
print(f"API 可达; 版本 {version}")
' <<<"$raw"
}

print_runtime_summary_lines() {
  local current_mode_summary current_mode_value current_mode_source configured_mode_value
  local runtime_policy_groups controller_runtime_summary_text

  current_mode_summary="$(current_mode_with_source)"
  current_mode_value="${current_mode_summary%%$'\t'*}"
  current_mode_source="${current_mode_summary#*$'\t'}"
  configured_mode_value="$(configured_mode || true)"
  [[ -n "$configured_mode_value" ]] || configured_mode_value="rule"
  runtime_policy_groups="$(runtime_policy_group_summary 2>/dev/null || true)"
  [[ -n "$runtime_policy_groups" ]] || runtime_policy_groups="未获取"
  controller_runtime_summary_text="$(controller_runtime_summary 2>/dev/null || true)"
  [[ -n "$controller_runtime_summary_text" ]] || controller_runtime_summary_text="未获取"

  echo "当前模式: ${current_mode_value}"
  echo "当前模式来源: ${current_mode_source}"
  echo "本地配置模式: ${configured_mode_value}"
  echo "运行态策略组: ${runtime_policy_groups}"
  echo "控制面运行态: ${controller_runtime_summary_text}"
}

print_controller_static_lines() {
  echo "外部 UI 名称: ${EXTERNAL_UI_NAME:-未设置}"
  echo "外部 UI 地址: ${EXTERNAL_UI_URL:-未设置}"
  echo "控制面 CORS Origins: $([[ -n "${CONTROLLER_CORS_ALLOW_ORIGINS:-}" ]] && echo "${CONTROLLER_CORS_ALLOW_ORIGINS}" || echo '未设置')"
  echo "控制面 CORS Private-Network: $([[ "${CONTROLLER_CORS_ALLOW_PRIVATE_NETWORK:-0}" == "1" ]] && echo '启用' || echo '关闭')"
  echo "控制面范围: ${CONTROLLER_SCOPE}"
}

print_network_access_lines() {
  local mode="${1:-status}"
  local dns_hijack_value
  local host_output_label
  local host_output_value

  if [[ "$mode" == "audit" ]]; then
    dns_hijack_value="$([[ "${DNS_HIJACK_ENABLED}" == "1" ]] && echo "${DNS_HIJACK_INTERFACES:-未配置}" || echo '关闭')"
    host_output_label="宿主机流量模式"
    host_output_value="$([[ "${PROXY_HOST_OUTPUT}" == "1" ]] && echo '透明接管(高风险)' || echo '默认直连 + localhost 显式代理')"
  else
    dns_hijack_value="$([[ "${DNS_HIJACK_ENABLED}" == "1" ]] && echo "${DNS_HIJACK_INTERFACES}" || echo '关闭')"
    host_output_label="宿主机流量"
    host_output_value="$([[ "$PROXY_HOST_OUTPUT" == "1" ]] && echo '透明接管(高风险)' || echo "默认直连；按需显式代理 http://127.0.0.1:${MIXED_PORT}")"
  fi

  echo "局域网旁路由入口: ${PROXY_INGRESS_INTERFACES:-未配置}"
  echo "局域网网段: ${LAN_CIDRS:-未设置}"
  echo "局域网禁止网段: ${LAN_DISALLOWED_CIDRS:-无}"
  echo "DNS 劫持入口: ${dns_hijack_value}"
  echo "${host_output_label}: ${host_output_value}"
  echo "显式代理认证: $([[ -n "${PROXY_AUTH_CREDENTIALS:-}" ]] && echo "启用 (${#PROXY_AUTH_CREDENTIALS_ARR[@]} 组账号)" || echo '关闭')"
  echo "显式代理免认证网段: $([[ -n "${SKIP_AUTH_PREFIXES:-}" ]] && echo "${SKIP_AUTH_PREFIXES}" || echo '无')"

  if [[ "$mode" != "audit" ]]; then
    echo "容器直连名单: ${BYPASS_CONTAINER_NAMES:-无}"
  fi
}

print_profile_summary_lines() {
  local mode="${1:-status}"

  if [[ "$mode" == "audit" ]]; then
    echo "当前模板: ${TEMPLATE_NAME:-unknown} ($(template_summary "${TEMPLATE_NAME:-unknown}"))"
    echo "规则预设: ${RULESET_PRESET:-$(default_rule_preset)} ($(rule_preset_summary "${RULESET_PRESET:-$(default_rule_preset)}"))"
    echo "IPv6 模式: $([[ "${ENABLE_IPV6:-0}" == "1" ]] && echo '启用' || echo '关闭')"
    return 0
  fi

  echo "模板: ${TEMPLATE_NAME:-$PROFILE_TEMPLATE} ($(template_summary "${TEMPLATE_NAME:-$PROFILE_TEMPLATE}"))"
  echo "规则预设: ${RULESET_PRESET:-$(default_rule_preset)} ($(rule_preset_summary "${RULESET_PRESET:-$(default_rule_preset)}"))"
  echo "IPv6: $([[ "${ENABLE_IPV6:-0}" == "1" ]] && echo '启用' || echo '关闭')"
}

print_count_summary_lines() {
  local mode="${1:-status}"
  local counts manual_enabled sub_enabled sub_total sub_ready

  if [[ "$mode" == "audit" ]]; then
    counts="$(readonly_node_counts)"
    echo "节点统计: 启用=${counts%%$'\t'*} 总计=${counts##*$'\t'}"
    return 0
  fi

  counts="$(manual_node_counts)"
  manual_enabled="${counts%%$'\t'*}"
  counts="${counts##*$'\t'}"
  echo "手动节点: 启用 ${manual_enabled} / 总计 ${counts}"

  counts="$(subscription_counts)"
  echo "订阅: 启用 ${counts%%$'\t'*} / 总计 ${counts##*$'\t'}"

  counts="$(subscription_provider_counts)"
  sub_enabled="${counts%%$'\t'*}"
  counts="${counts#*$'\t'}"
  sub_total="${counts%%$'\t'*}"
  sub_ready="${counts##*$'\t'}"
  echo "订阅 provider: 启用 ${sub_enabled} / 总计 ${sub_total}"
  echo "订阅缓存: 就绪 ${sub_ready} / 总计 ${sub_total}"
}

status_next_step() {
  local service_state="${1:-inactive}"
  local manual_enabled="${2:-0}"
  local subscription_enabled="${3:-0}"
  local provider_enabled="${4:-0}"
  local provider_ready="${5:-0}"

  if [[ "$manual_enabled" == "0" && "$subscription_enabled" == "0" ]]; then
    echo "导入节点: mihomo import-links 或添加订阅: mihomo add-subscription"
  elif [[ "$provider_enabled" -gt 0 && "$provider_ready" -lt "$provider_enabled" ]]; then
    echo "更新订阅: mihomo update-subscriptions"
  elif [[ "$manual_enabled" == "0" && "$subscription_enabled" != "0" ]]; then
    echo "更新订阅: mihomo update-subscriptions"
  elif [[ "$service_state" != "active" ]]; then
    echo "启动服务: mihomo start"
  else
    echo "宿主机默认直连；局域网设备把网关和 DNS 指向 NAS 后即可走旁路由"
  fi
}

status_next_step_for_service_state() {
  local service_state="${1:-inactive}"
  local manual_counts subscription_summary provider_counts
  local manual_enabled subscription_enabled provider_enabled provider_ready

  manual_counts="$(manual_node_counts)"
  subscription_summary="$(subscription_counts)"
  provider_counts="$(subscription_provider_counts)"

  manual_enabled="${manual_counts%%$'\t'*}"
  subscription_enabled="${subscription_summary%%$'\t'*}"
  provider_enabled="${provider_counts%%$'\t'*}"
  provider_counts="${provider_counts#*$'\t'}"
  provider_ready="${provider_counts##*$'\t'}"

  status_next_step "$service_state" "$manual_enabled" "$subscription_enabled" "$provider_enabled" "$provider_ready"
}

print_status_warnings_and_footer() {
  local profile_name="${1:-}"

  if template_is_deprecated "$profile_name"; then
    warn "当前模板仅兼容保留；本项目当前只承诺 Debian NAS 的 IPv4 旁路由。"
  fi
  if [[ "${CONTROLLER_SCOPE:-仅宿主机}" != "仅宿主机" ]]; then
    warn "控制面当前已开放到局域网；更推荐保持 CONTROLLER_BIND_ADDRESS=127.0.0.1。"
  fi
  if [[ "${PROXY_HOST_OUTPUT:-0}" == "1" ]]; then
    warn "当前已接管宿主机外连；这可能影响 tailscaled、cloudflared、SSH 反向隧道等守护进程。"
  fi
  echo "定时重启: ${RESTART_INTERVAL_HOURS:-0}h"
  echo "Alpha 自动更新: $([[ "${ALPHA_AUTO_UPDATE:-0}" == "1" ]] && echo "${ALPHA_UPDATE_ONCALENDAR}" || echo '关闭')"
}

print_status_sync_and_port_lines() {
  if [[ "${MANAGER_SYNC_ENABLED:-0}" == "1" ]]; then
    echo "本机源码同步: 启用；每 ${MANAGER_SYNC_INTERVAL_MINUTES:-1} 分钟从 ${MANAGER_SYNC_SOURCE:-未知来源} 同步"
  else
    echo "本机源码同步: 关闭"
  fi
  echo "Mixed/TProxy/DNS: ${MIXED_PORT}/${TPROXY_PORT}/${DNS_PORT}"
}

print_status_access_entry_lines() {
  local controller_url

  controller_url="http://${CONTROLLER_HOST}:${CONTROLLER_PORT}/ui/"
  echo "WebUI: ${controller_url}"
  echo "控制面密钥: 已隐藏；如需查看执行: mihomo show-secret"
}

status_overview_snapshot() {
  local version="未安装"
  local service_state="inactive"
  local service_enable="disabled"

  [[ -x "$MIHOMO_BIN" ]] && version="$("$MIHOMO_BIN" -v 2>/dev/null | head -n 1 || echo "$MIHOMO_BIN")"
  [[ -n "$version" ]] || version="未安装"
  service_is_active && service_state="active"
  service_is_enabled && service_enable="enabled"
  printf '%s\t%s\t%s\n' "$version" "$service_state" "$service_enable"
}

print_status_overview_lines() {
  local version="${1:-未安装}"
  local service_state="${2:-inactive}"
  local service_enable="${3:-disabled}"

  echo
  echo "Mihomo 管理器 v${SCRIPT_VERSION}"
  echo "核心版本: ${version}"
  echo "服务状态: ${service_state}"
  echo "开机自启: ${service_enable}"
}

print_runtime_audit_overview_lines() {
  local active_state="${1:-unknown}"
  local sub_state="${2:-unknown}"
  local enabled_state="${3:-unknown}"
  local main_pid="${4:-0}"
  local active_since="${5:-unknown}"
  local n_restarts="${6:-0}"
  local memory_current="${7:-0}"
  local memory_peak="${8:-0}"
  local cpu_nsec="${9:-0}"

  echo "== 运行审计 =="
  echo "服务状态: ${active_state}"
  echo "运行子状态: ${sub_state}"
  echo "开机自启: ${enabled_state}"
  echo "主进程 PID: ${main_pid}"
  echo "本次启动时间: ${active_since}"
  echo "自 systemd 接管后的重启次数: ${n_restarts}"
  echo "当前内存占用(字节): ${memory_current}"
  echo "历史峰值内存(字节): ${memory_peak}"
  echo "累计 CPU 时间(ns): ${cpu_nsec}"
  echo "端口监听: mixed=${MIXED_PORT} tproxy=${TPROXY_PORT} dns=${DNS_PORT} controller=${CONTROLLER_PORT}"
}

print_runtime_audit_probe_lines() {
  local proxy_probe="${1:-fail}"
  local controller_probe="${2:-fail}"
  local tproxy_packets="${3:-0}"
  local dns_hijack_packets="${4:-0}"
  local lan_activity_summary="${5:-未获取}"

  echo "localhost 显式代理探测: ${proxy_probe}"
  echo "本机 WebUI 探测: ${controller_probe}"
  echo "局域网透明代理命中包数: ${tproxy_packets}"
  echo "DNS 劫持命中包数: ${dns_hijack_packets}"
  echo "旁路由流量摘要: ${lan_activity_summary}"
}

print_runtime_audit_alert_lines() {
  local warn_count="${1:-0}"
  local err_count="${2:-0}"
  local trigger_update="${3:-disabled}"
  local trigger_restart="${4:-disabled}"

  echo "过去 24 小时 warning 数: ${warn_count}"
  echo "过去 24 小时 error 数: ${err_count}"
  echo "下次 Alpha 自动更新: ${trigger_update}"
  echo "下次定时重启: ${trigger_restart}"
}
