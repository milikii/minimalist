#!/usr/bin/env bash

PREV_CORE_BIN="${PREV_CORE_BIN:-${MIHOMO_BIN}.prev}"

current_secret() {
  if [[ -f "$CONFIG_FILE" ]]; then
    awk -F '"' '/^secret:/ { print $2; exit }' "$CONFIG_FILE"
  fi
  return 0
}

ensure_mihomo_user() {
  if ! user_exists "$MIHOMO_USER"; then
    useradd -r -M -s /usr/sbin/nologin "$MIHOMO_USER"
    ok "已创建用户 ${MIHOMO_USER}"
  fi
}

ensure_permissions() {
  ensure_mihomo_user
  ensure_layout
  chown -R "${MIHOMO_USER}:${MIHOMO_USER}" "$MIHOMO_DIR"
  chmod 750 "$MIHOMO_DIR" "$RULES_DIR" "$PROVIDER_DIR" "$UI_DIR" "$STATE_DIR"
  [[ -f "$CONFIG_FILE" ]] && chmod 640 "$CONFIG_FILE"
  [[ -f "$PROVIDER_FILE" ]] && chmod 640 "$PROVIDER_FILE"
  [[ -f "$RENDERED_RULES_FILE" ]] && chmod 640 "$RENDERED_RULES_FILE"
  [[ -f "$NODES_STATE_FILE" ]] && chmod 640 "$NODES_STATE_FILE"
  [[ -f "$RULES_STATE_FILE" ]] && chmod 640 "$RULES_STATE_FILE"
  [[ -f "$SETTINGS_ENV" ]] && chown root:"$MIHOMO_USER" "$SETTINGS_ENV"
  [[ -f "$ROUTER_ENV" ]] && chown root:"$MIHOMO_USER" "$ROUTER_ENV"
  [[ -f "$COUNTRY_MMDB" ]] && chmod 644 "$COUNTRY_MMDB"
  [[ -f "${MIHOMO_DIR}/cache.db" ]] && chmod 640 "${MIHOMO_DIR}/cache.db"
  return 0
}

render_provider_file() {
  require_statectl
  python3 "$STATECTL" render-provider "$NODES_STATE_FILE" "$PROVIDER_FILE"
}

render_rules_file() {
  require_statectl
  python3 "$STATECTL" render-rules "$RULES_STATE_FILE" "$RENDERED_RULES_FILE"
}

validate_rule_targets() {
  require_statectl
  local output
  if ! output="$(python3 "$STATECTL" validate-rule-targets "$RULES_STATE_FILE" "$NODES_STATE_FILE" 2>&1)"; then
    [[ -n "$output" ]] && printf '%s\n' "$output" >&2
    die "存在自定义规则指向不存在或未启用节点"
  fi
}

render_rules_block() {
  if [[ -f "$RENDERED_RULES_FILE" ]]; then
    awk 'NF && $0 !~ /^[[:space:]]*#/' "$RENDERED_RULES_FILE"
  fi
  return 0
}

render_config() {
  require_root
  ensure_layout
  ensure_permissions
  load_settings
  load_router_env
  validate_rule_targets
  render_provider_file
  render_rules_file

  local secret
  local enabled_count
  local lan_cidrs
  local config_mode
  local allowed_cidr
  local -a lan_allowed_cidrs_arr=()

  secret="$(current_secret)"
  [[ -n "$secret" ]] || secret="$(random_secret)"
  enabled_count="$(node_enabled_count)"
  config_mode="${CONFIG_MODE:-rule}"
  lan_cidrs="${LAN_CIDRS:-192.168.2.0/24}"
  read -r -a lan_allowed_cidrs_arr <<< "${lan_cidrs}"
  lan_allowed_cidrs_arr+=("127.0.0.0/8")

  cat >"$CONFIG_FILE" <<EOF
mixed-port: ${MIXED_PORT}
tproxy-port: ${TPROXY_PORT}
allow-lan: true
bind-address: "*"
lan-allowed-ips:
EOF
  for allowed_cidr in "${lan_allowed_cidrs_arr[@]}"; do
    printf '  - %s\n' "$allowed_cidr" >>"$CONFIG_FILE"
  done
  cat >>"$CONFIG_FILE" <<EOF
mode: ${config_mode}
log-level: info
ipv6: false
unified-delay: true
tcp-concurrent: true
find-process-mode: off
geodata-mode: false
geo-auto-update: false
geo-update-interval: 24
geox-url:
  mmdb: "https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/country.mmdb"
  geoip: "https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/geoip.dat"
  geosite: "https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/geosite.dat"

external-controller: ${CONTROLLER_BIND_ADDRESS}:${CONTROLLER_PORT}
secret: "${secret}"
external-ui: ${UI_DIR}

profile:
  store-selected: true
  store-fake-ip: true

dns:
  enable: true
  listen: 0.0.0.0:${DNS_PORT}
  ipv6: false
  use-hosts: true
  use-system-hosts: true
  respect-rules: false
  prefer-h3: false
  enhanced-mode: fake-ip
  fake-ip-range: 198.18.0.1/16
  fake-ip-filter:
    - "*.lan"
    - "*.local"
    - "+.stun.*.*"
    - "localhost.ptlogin2.qq.com"
  nameserver:
    - https://doh.pub/dns-query
    - https://dns.alidns.com/dns-query
  fallback:
    - https://1.1.1.1/dns-query
    - https://8.8.8.8/dns-query
  proxy-server-nameserver:
    - https://doh.pub/dns-query
    - https://dns.alidns.com/dns-query
  fallback-filter:
    geoip: true
    geoip-code: CN
    ipcidr:
      - 240.0.0.0/4
      - 0.0.0.0/32

proxies: []
EOF

  if [[ "$enabled_count" -gt 0 ]]; then
    cat >>"$CONFIG_FILE" <<'EOF'

proxy-providers:
  manual:
    type: file
    path: ./proxy_providers/manual.txt
    health-check:
      enable: true
      url: "https://cp.cloudflare.com/generate_204"
      interval: 300
      timeout: 5000
      lazy: true

proxy-groups:
  - name: "PROXY"
    type: select
    proxies:
      - DIRECT
      - AUTO
    use:
      - manual

  - name: "AUTO"
    type: url-test
    url: "https://cp.cloudflare.com/generate_204"
    interval: 300
    tolerance: 80
    lazy: true
    use:
      - manual
EOF
  else
    cat >>"$CONFIG_FILE" <<'EOF'

proxy-groups:
  - name: "PROXY"
    type: select
    proxies:
      - DIRECT
EOF
  fi

  cat >>"$CONFIG_FILE" <<'EOF'

rules:
EOF

  local custom_rule
  while IFS= read -r custom_rule; do
    [[ -n "$custom_rule" ]] || continue
    printf '  - %s\n' "$custom_rule" >>"$CONFIG_FILE"
  done < <(render_rules_block)

  cat >>"$CONFIG_FILE" <<'EOF'
  - PROCESS-NAME,mihomo,DIRECT
  - GEOIP,CN,DIRECT
  - MATCH,PROXY
EOF

  chown "${MIHOMO_USER}:${MIHOMO_USER}" "$CONFIG_FILE"
  chmod 640 "$CONFIG_FILE"
  ok "已生成 ${CONFIG_FILE}"
}

write_sysctl() {
  require_root
  cat >"$ROUTER_SYSCTL" <<'EOF'
net.ipv4.ip_forward = 1
net.ipv4.conf.all.route_localnet = 1
net.ipv4.conf.default.rp_filter = 2
net.ipv4.conf.all.rp_filter = 2
EOF
  sysctl --system >/dev/null
  ok "已写入 ${ROUTER_SYSCTL}"
}

write_service() {
  require_root
  cat >"$SYSTEMD_UNIT" <<EOF
[Unit]
Description=Mihomo Side Router
After=network-online.target docker.service
Wants=network-online.target
ConditionPathExists=${CONFIG_FILE}

[Service]
Type=simple
User=${MIHOMO_USER}
Group=${MIHOMO_USER}
ExecStartPre=+${MANAGER_BIN} apply-rules
ExecStart=${MIHOMO_BIN} -d ${MIHOMO_DIR}
ExecReload=+${MANAGER_BIN} apply-rules
ExecStopPost=+${MANAGER_BIN} clear-rules
Restart=on-failure
RestartSec=5
LimitNOFILE=1048576
AmbientCapabilities=CAP_NET_ADMIN CAP_NET_BIND_SERVICE CAP_NET_RAW
CapabilityBoundingSet=CAP_NET_ADMIN CAP_NET_BIND_SERVICE CAP_NET_RAW
NoNewPrivileges=true
PrivateTmp=true
ProtectHome=true
ProtectSystem=full
ReadWritePaths=${MIHOMO_DIR}

[Install]
WantedBy=multi-user.target
EOF
  systemctl_cmd daemon-reload
  ok "已写入 ${SYSTEMD_UNIT}"
}

write_restart_units() {
  local hours="${1:-0}"
  cat >"$RESTART_SERVICE_UNIT" <<'EOF'
[Unit]
Description=Restart Mihomo Service

[Service]
Type=oneshot
ExecStart=/bin/systemctl restart mihomo.service
EOF
  cat >"$RESTART_TIMER_UNIT" <<EOF
[Unit]
Description=Periodic Mihomo Restart Timer

[Timer]
OnBootSec=15min
OnUnitActiveSec=${hours}h
Persistent=true
Unit=mihomo-restart.service

[Install]
WantedBy=timers.target
EOF
}

configure_restart_timer() {
  require_root
  local hours="$1"
  upsert_env_var "$SETTINGS_ENV" "RESTART_INTERVAL_HOURS" "$hours"
  if [[ "$hours" =~ ^[0-9]+$ ]] && [[ "$hours" -gt 0 ]]; then
    write_restart_units "$hours"
    systemctl_cmd daemon-reload
    systemctl_cmd enable --now mihomo-restart.timer
    ok "已启用定时重启: 每 ${hours} 小时"
  else
    systemctl_cmd disable --now mihomo-restart.timer >/dev/null 2>&1 || true
    rm -f "$RESTART_SERVICE_UNIT" "$RESTART_TIMER_UNIT"
    systemctl_cmd daemon-reload
    ok "已关闭定时重启"
  fi
}

write_update_units() {
  local schedule="$1"
  cat >"$UPDATE_SERVICE_UNIT" <<EOF
[Unit]
Description=Update Mihomo Alpha Core
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
ExecStart=${MANAGER_BIN} update-alpha --quiet
EOF
  cat >"$UPDATE_TIMER_UNIT" <<EOF
[Unit]
Description=Auto Update Mihomo Alpha Timer

[Timer]
OnCalendar=${schedule}
Persistent=true
Unit=mihomo-alpha-update.service

[Install]
WantedBy=timers.target
EOF
}

configure_alpha_update() {
  require_root
  local enabled="$1"
  local schedule="$2"
  upsert_env_var "$SETTINGS_ENV" "ALPHA_AUTO_UPDATE" "$enabled"
  upsert_env_var "$SETTINGS_ENV" "ALPHA_UPDATE_ONCALENDAR" "$schedule"
  if [[ "$enabled" == "1" ]]; then
    write_update_units "$schedule"
    systemctl_cmd daemon-reload
    systemctl_cmd enable --now mihomo-alpha-update.timer
    ok "已启用 Alpha 自动更新: ${schedule}"
  else
    systemctl_cmd disable --now mihomo-alpha-update.timer >/dev/null 2>&1 || true
    rm -f "$UPDATE_SERVICE_UNIT" "$UPDATE_TIMER_UNIT"
    systemctl_cmd daemon-reload
    ok "已关闭 Alpha 自动更新"
  fi
}

download_core_to_temp() {
  local channel="$1"
  local output_file="$2"
  local arch
  local release_url
  local tmp_dir
  local json_file
  local json

  case "$(uname -m)" in
    x86_64) arch="amd64" ;;
    aarch64) arch="arm64" ;;
    armv7l) arch="armv7" ;;
    *) die "不支持的架构: $(uname -m)" ;;
  esac
  case "$channel" in
    alpha) release_url="https://api.github.com/repos/MetaCubeX/mihomo/releases" ;;
    stable|latest) release_url="https://api.github.com/repos/MetaCubeX/mihomo/releases/latest" ;;
    *) die "install-binary 仅支持 alpha|stable" ;;
  esac

  tmp_dir="$(mktemp -d)"
  trap 'rm -rf "$tmp_dir"' RETURN
  json="$(curl_cmd -fsSL "$release_url")" || die "拉取 release 信息失败"
  json_file="${tmp_dir}/release.json"
  printf '%s' "$json" >"$json_file"

  local asset_url
  asset_url="$(
    python3 - "$arch" "$channel" "$json_file" <<'PY'
import json
import sys
arch, channel, path = sys.argv[1:4]
with open(path, "r", encoding="utf-8") as f:
    data = json.load(f)
releases = data if isinstance(data, list) else [data]
def pick(assets):
    prefixes = [f"mihomo-linux-{arch}-compatible-", f"mihomo-linux-{arch}-"]
    for prefix in prefixes:
        for asset in assets:
            name = asset.get("name", "")
            if name.startswith(prefix) and name.endswith(".gz"):
                return asset["browser_download_url"]
    return None
for rel in releases:
    if channel == "alpha" and not rel.get("prerelease"):
        continue
    url = pick(rel.get("assets", []))
    if url:
        print(url)
        raise SystemExit(0)
raise SystemExit("no matching asset found")
PY
  )" || die "没有找到匹配当前架构的 Mihomo 下载资产"

  info "下载 ${asset_url}"
  curl_cmd -fL --progress-bar -o "${tmp_dir}/mihomo.gz" "$asset_url" || die "Mihomo 下载失败"
  gunzip "${tmp_dir}/mihomo.gz"
  install -m 0755 "${tmp_dir}/mihomo" "$output_file"
  rm -rf "$tmp_dir"
  trap - RETURN
}

install_binary() {
  require_root
  local channel="${1:-alpha}"
  local quiet="${2:-}"
  local tmp_core
  tmp_core="$(mktemp)"
  rm -f "$tmp_core"
  download_core_to_temp "$channel" "$tmp_core"
  if [[ -f "$CONFIG_FILE" ]]; then
    "$tmp_core" -t -d "$MIHOMO_DIR" >/tmp/mihomo-core-test.log 2>&1 || {
      sed -n '1,160p' /tmp/mihomo-core-test.log >&2
      rm -f "$tmp_core"
      die "新内核未通过配置检查"
    }
  fi
  [[ -x "$MIHOMO_BIN" ]] && cp -f "$MIHOMO_BIN" "$PREV_CORE_BIN"
  install -m 0755 "$tmp_core" "$MIHOMO_BIN"
  rm -f "$tmp_core"
  [[ "$quiet" == "--quiet" ]] || ok "已安装 $("$MIHOMO_BIN" -v 2>/dev/null | head -n 1)"
}

rollback_core() {
  require_root
  [[ -f "$PREV_CORE_BIN" ]] || die "未找到回滚核心: ${PREV_CORE_BIN}"
  install -m 0755 "$PREV_CORE_BIN" "$MIHOMO_BIN"
  restart_service_if_active
  ok "已回滚到上一个内核版本"
}

install_country_mmdb() {
  require_root
  curl_cmd -fL --progress-bar -o "$COUNTRY_MMDB" https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/country.mmdb
  chown "${MIHOMO_USER}:${MIHOMO_USER}" "$COUNTRY_MMDB"
  chmod 644 "$COUNTRY_MMDB"
}

install_webui() {
  require_root
  local ui_name="${1:-zashboard}"
  local ui_url
  local tmp
  local src
  case "$ui_name" in
    zashboard) ui_url="https://github.com/Zephyruso/zashboard/archive/refs/heads/gh-pages.zip" ;;
    *) die "当前仅支持 zashboard" ;;
  esac
  tmp="$(mktemp -d)"
  trap 'rm -rf "$tmp"' RETURN
  mkdir -p "$UI_DIR"
  info "下载 WebUI: ${ui_name}"
  curl_cmd -fL --progress-bar -o "${tmp}/ui.zip" "$ui_url"
  unzip -q "${tmp}/ui.zip" -d "$tmp"
  src="$(find "$tmp" -maxdepth 1 -mindepth 1 -type d | head -n 1)"
  [[ -n "$src" ]] || die "未找到解压后的 WebUI 目录"
  find "$UI_DIR" -mindepth 1 -maxdepth 1 -exec rm -rf {} +
  cp -a "${src}/." "$UI_DIR/"
  chown -R "${MIHOMO_USER}:${MIHOMO_USER}" "$UI_DIR"
  rm -rf "$tmp"
  trap - RETURN
  ok "WebUI 已安装到 ${UI_DIR}"
}

install_project() {
  require_root
  local src_root="$1"
  local install_root="/usr/local/lib/mihomo-manager"
  rm -rf "$install_root"
  mkdir -p /usr/local/lib
  cp -a "$src_root" "$install_root"
  find "$install_root" -type d -name '__pycache__' -prune -exec rm -rf {} +
  find "$install_root" -type f \( -name '*.bak.*' -o -name '*.pyc' \) -delete
  chmod +x "$install_root/mihomo" "$install_root/scripts/statectl.py"
  ln -sf "$install_root/mihomo" "$MANAGER_BIN"
  ln -sf "$install_root/mihomo" "$COMPAT_MANAGER_BIN"
  ok "已安装管理命令到 ${MANAGER_BIN}"
}

delete_jump() {
  local table="$1"
  shift
  while ipt -t "$table" -C "$@" >/dev/null 2>&1; do
    ipt -t "$table" -D "$@"
  done
}

ensure_chain() {
  local table="$1"
  local chain="$2"
  if ipt -t "$table" -S "$chain" >/dev/null 2>&1; then
    ipt -t "$table" -F "$chain"
  else
    ipt -t "$table" -N "$chain"
  fi
}

ip_rule_del() {
  while ip -4 rule del fwmark "$ROUTE_MARK_DEC" table "$ROUTE_TABLE" priority "$ROUTE_PRIORITY" 2>/dev/null; do :; done
}

resolve_bypass_container_ips() {
  CONTAINER_BYPASS_IPS_ARR=()
  [[ ${#BYPASS_CONTAINER_NAMES_ARR[@]} -gt 0 ]] || return 0
  have docker || return 0
  local name
  local ip
  local ips=()
  for name in "${BYPASS_CONTAINER_NAMES_ARR[@]}"; do
    if ! docker inspect "$name" >/dev/null 2>&1; then
      continue
    fi
    while read -r ip; do
      [[ -n "$ip" ]] && ips+=("${ip}/32")
    done < <(docker inspect "$name" --format '{{range .NetworkSettings.Networks}}{{println .IPAddress}}{{end}}' 2>/dev/null | awk 'NF')
  done
  if [[ ${#ips[@]} -gt 0 ]]; then
    mapfile -t CONTAINER_BYPASS_IPS_ARR < <(printf '%s\n' "${ips[@]}" | sort -u)
  fi
}

require_firewall_support() {
  have iptables || die "未找到 iptables"
  iptables -w 5 -t mangle -S >/dev/null 2>&1 || die "当前系统无法正常操作 iptables/nft 后端"
}

clear_rules() {
  require_root
  load_router_env
  delete_jump mangle PREROUTING -j MIHOMO_PRE
  delete_jump mangle OUTPUT -j MIHOMO_OUT
  delete_jump nat PREROUTING -j MIHOMO_DNS
  delete_jump nat OUTPUT -j MIHOMO_DNS_OUT
  ipt -t mangle -F MIHOMO_PRE 2>/dev/null || true
  ipt -t mangle -X MIHOMO_PRE 2>/dev/null || true
  ipt -t mangle -F MIHOMO_PRE_HANDLE 2>/dev/null || true
  ipt -t mangle -X MIHOMO_PRE_HANDLE 2>/dev/null || true
  ipt -t mangle -F MIHOMO_OUT 2>/dev/null || true
  ipt -t mangle -X MIHOMO_OUT 2>/dev/null || true
  ipt -t nat -F MIHOMO_DNS 2>/dev/null || true
  ipt -t nat -X MIHOMO_DNS 2>/dev/null || true
  ipt -t nat -F MIHOMO_DNS_HANDLE 2>/dev/null || true
  ipt -t nat -X MIHOMO_DNS_HANDLE 2>/dev/null || true
  ipt -t nat -F MIHOMO_DNS_OUT 2>/dev/null || true
  ipt -t nat -X MIHOMO_DNS_OUT 2>/dev/null || true
  ip_rule_del
  ip -4 route flush table "$ROUTE_TABLE" 2>/dev/null || true
}

apply_rules() {
  require_root
  load_router_env
  require_firewall_support
  resolve_bypass_container_ips
  ensure_enabled_nodes
  [[ ${#PROXY_INGRESS_IFACES_ARR[@]} -gt 0 ]] || die "PROXY_INGRESS_INTERFACES 不能为空"
  if [[ "$DNS_HIJACK_ENABLED" == "1" && ${#DNS_HIJACK_IFACES_ARR[@]} -eq 0 ]]; then
    die "DNS_HIJACK_INTERFACES 不能为空"
  fi
  clear_rules
  ensure_chain mangle MIHOMO_PRE
  ensure_chain mangle MIHOMO_PRE_HANDLE
  ensure_chain mangle MIHOMO_OUT
  ensure_chain nat MIHOMO_DNS
  ensure_chain nat MIHOMO_DNS_HANDLE
  local iface
  for iface in "${PROXY_INGRESS_IFACES_ARR[@]}"; do
    ipt -t mangle -A MIHOMO_PRE -i "$iface" -j MIHOMO_PRE_HANDLE
  done
  ipt -t mangle -A MIHOMO_PRE -j RETURN
  if [[ "$DNS_HIJACK_ENABLED" == "1" ]]; then
    ipt -t mangle -A MIHOMO_PRE_HANDLE -p udp --dport 53 -j RETURN
    ipt -t mangle -A MIHOMO_PRE_HANDLE -p tcp --dport 53 -j RETURN
  fi
  local cidr
  for cidr in "${RESERVED_DST_CIDRS_ARR[@]}"; do
    ipt -t mangle -A MIHOMO_PRE_HANDLE -d "$cidr" -j RETURN
    ipt -t mangle -A MIHOMO_OUT -d "$cidr" -j RETURN
  done
  for cidr in "${BYPASS_DST_CIDRS_ARR[@]}"; do
    [[ -n "$cidr" ]] || continue
    ipt -t mangle -A MIHOMO_PRE_HANDLE -d "$cidr" -j RETURN
    ipt -t mangle -A MIHOMO_OUT -d "$cidr" -j RETURN
  done
  for cidr in "${BYPASS_SRC_CIDRS_ARR[@]}"; do
    [[ -n "$cidr" ]] || continue
    ipt -t mangle -A MIHOMO_PRE_HANDLE -s "$cidr" -j RETURN
    ipt -t nat -A MIHOMO_DNS_HANDLE -s "$cidr" -j RETURN
  done
  for cidr in "${CONTAINER_BYPASS_IPS_ARR[@]}"; do
    ipt -t mangle -A MIHOMO_PRE_HANDLE -s "$cidr" -j RETURN
    ipt -t nat -A MIHOMO_DNS_HANDLE -s "$cidr" -j RETURN
  done
  ipt -t mangle -A MIHOMO_PRE_HANDLE -p tcp -j TPROXY --on-port "$TPROXY_PORT" --tproxy-mark "${ROUTE_MARK}/${ROUTE_MASK}"
  ipt -t mangle -A MIHOMO_PRE_HANDLE -p udp -j TPROXY --on-port "$TPROXY_PORT" --tproxy-mark "${ROUTE_MARK}/${ROUTE_MASK}"
  local mihomo_uid
  mihomo_uid="$(id -u "$MIHOMO_USER")"
  ipt -t mangle -A MIHOMO_OUT -m owner --uid-owner "$mihomo_uid" -j RETURN
  # Do not intercept reply-direction packets for inbound connections.
  # This keeps locally hosted services like SSH from having their response
  # packets transparently proxied on the way back to external clients.
  ipt -t mangle -A MIHOMO_OUT -m conntrack --ctdir REPLY -j RETURN
  for cidr in "${BYPASS_UIDS_ARR[@]}"; do
    [[ -n "$cidr" ]] || continue
    ipt -t mangle -A MIHOMO_OUT -m owner --uid-owner "$cidr" -j RETURN
  done
  ipt -t mangle -A MIHOMO_OUT -p tcp -j MARK --set-mark "$ROUTE_MARK_DEC"
  ipt -t mangle -A MIHOMO_OUT -p udp -j MARK --set-mark "$ROUTE_MARK_DEC"
  if [[ "$DNS_HIJACK_ENABLED" == "1" ]]; then
    for iface in "${DNS_HIJACK_IFACES_ARR[@]}"; do
      ipt -t nat -A MIHOMO_DNS -i "$iface" -j MIHOMO_DNS_HANDLE
    done
    ipt -t nat -A MIHOMO_DNS -j RETURN
    ipt -t nat -A MIHOMO_DNS_HANDLE -p udp --dport 53 -j REDIRECT --to-ports "$DNS_PORT"
    ipt -t nat -A MIHOMO_DNS_HANDLE -p tcp --dport 53 -j REDIRECT --to-ports "$DNS_PORT"
    if ! ipt -t nat -C PREROUTING -j MIHOMO_DNS >/dev/null 2>&1; then
      ipt -t nat -A PREROUTING -j MIHOMO_DNS
    fi
  fi
  if ! ipt -t mangle -C PREROUTING -j MIHOMO_PRE >/dev/null 2>&1; then
    ipt -t mangle -A PREROUTING -j MIHOMO_PRE
  fi
  if [[ "$PROXY_HOST_OUTPUT" == "1" ]]; then
    guard_host_output_proxy_conflicts
    print_host_output_proxy_warning
    if ! ipt -t mangle -C OUTPUT -j MIHOMO_OUT >/dev/null 2>&1; then
      ipt -t mangle -A OUTPUT -j MIHOMO_OUT
    fi
  fi
  ip_rule_del
  ip -4 route replace local 0.0.0.0/0 dev lo table "$ROUTE_TABLE"
  ip -4 rule add fwmark "$ROUTE_MARK_DEC" table "$ROUTE_TABLE" priority "$ROUTE_PRIORITY"
  ok "已应用 Mihomo 路由规则"
}

port_in_use_by_other() {
  local port="$1"
  local output
  output="$(ss_cmd -lntup 2>/dev/null | grep -E "[:.]${port}[[:space:]]" || true)"
  [[ -z "$output" ]] && return 1
  if grep -Eq 'mihomo-core|mihomo' <<<"$output"; then
    return 1
  fi
  printf '%s\n' "$output"
  return 0
}

validate_ports() {
  local port
  for port in "$MIXED_PORT" "$TPROXY_PORT" "$DNS_PORT" "$CONTROLLER_PORT"; do
    if port_in_use_by_other "$port" >/tmp/mihomo-port-check.log; then
      sed -n '1,40p' /tmp/mihomo-port-check.log >&2
      die "端口 ${port} 已被其他进程占用"
    fi
  done
}

configure_ports() {
  require_root
  load_router_env
  local input
  read -rp "Mixed 端口 [${MIXED_PORT}]: " input
  upsert_env_var "$ROUTER_ENV" "MIXED_PORT" "${input:-$MIXED_PORT}"
  read -rp "TProxy 端口 [${TPROXY_PORT}]: " input
  upsert_env_var "$ROUTER_ENV" "TPROXY_PORT" "${input:-$TPROXY_PORT}"
  read -rp "DNS 端口 [${DNS_PORT}]: " input
  upsert_env_var "$ROUTER_ENV" "DNS_PORT" "${input:-$DNS_PORT}"
  read -rp "控制面板端口 [${CONTROLLER_PORT}]: " input
  upsert_env_var "$ROUTER_ENV" "CONTROLLER_PORT" "${input:-$CONTROLLER_PORT}"
  load_router_env
  validate_ports
  render_config
  restart_service_if_active
}

config_test() {
  require_root
  [[ -x "$MIHOMO_BIN" ]] || die "未找到内核 ${MIHOMO_BIN}"
  [[ -f "$CONFIG_FILE" ]] || die "未找到配置 ${CONFIG_FILE}"
  local log_file
  log_file="$(mktemp)"
  if "$MIHOMO_BIN" -t -d "$MIHOMO_DIR" >"$log_file" 2>&1; then
    ok "配置检查通过"
  else
    warn "配置检查失败:"
    sed -n '1,160p' "$log_file"
    rm -f "$log_file"
    return 1
  fi
  rm -f "$log_file"
}

healthcheck() {
  require_root
  load_router_env
  local failed=0
  service_is_active || { echo "service: inactive"; failed=1; }
  [[ -f "$COUNTRY_MMDB" ]] || { echo "geo: missing Country.mmdb"; failed=1; }
  ss_cmd -lntup 2>/dev/null | grep -qE "[:.]${MIXED_PORT}[[:space:]]" || { echo "port: mixed ${MIXED_PORT} not listening"; failed=1; }
  ss_cmd -lntup 2>/dev/null | grep -qE "[:.]${TPROXY_PORT}[[:space:]]" || { echo "port: tproxy ${TPROXY_PORT} not listening"; failed=1; }
  ss_cmd -lntup 2>/dev/null | grep -qE "[:.]${DNS_PORT}[[:space:]]" || { echo "port: dns ${DNS_PORT} not listening"; failed=1; }
  ss_cmd -lntup 2>/dev/null | grep -qE "[:.]${CONTROLLER_PORT}[[:space:]]" || { echo "port: controller ${CONTROLLER_PORT} not listening"; failed=1; }
  curl_cmd --noproxy '*' -fsS --max-time 10 "http://127.0.0.1:${CONTROLLER_PORT}/ui/" >/tmp/mihomo-health-ui.html 2>/dev/null || {
    echo "webui: unavailable"
    failed=1
  }
  curl_cmd --noproxy '*' -fsS --max-time 10 -x "http://127.0.0.1:${MIXED_PORT}" https://cp.cloudflare.com/generate_204 >/tmp/mihomo-health-proxy.out 2>/dev/null || {
    echo "proxy: localhost mixed ${MIXED_PORT} unavailable"
    failed=1
  }
  if [[ "$failed" -eq 0 ]]; then
    ok "健康检查通过"
    return 0
  fi
  return 1
}

diagnose() {
  require_root
  load_settings
  load_router_env
  echo "== status =="
  systemctl_cmd status mihomo --no-pager || true
  echo
  echo "== timers =="
  systemctl_cmd status mihomo-alpha-update.timer mihomo-restart.timer --no-pager 2>/dev/null || true
  echo
  echo "== listeners =="
  ss_cmd -lntup 2>/dev/null | grep -E "[:.](${MIXED_PORT}|${TPROXY_PORT}|${DNS_PORT}|${CONTROLLER_PORT})[[:space:]]" || true
  echo
  echo "== config summary =="
  printf 'mode=%s\n' "$(awk '/^mode:/ {print $2; exit}' "$CONFIG_FILE" 2>/dev/null || echo unknown)"
  printf 'enabled_nodes=%s\n' "$(node_enabled_count)"
  printf 'core_channel=%s\n' "${CORE_CHANNEL:-alpha}"
  echo
  echo "== recent logs =="
  journalctl_cmd -u mihomo -n 60 --no-pager || true
}

audit_installation() {
  require_root
  load_settings_readonly
  load_router_env_readonly
  local status=0
  local tmpdir
  tmpdir="$(mktemp -d)"
  trap 'rm -rf "$tmpdir"' RETURN

  echo "== audit =="
  [[ -f "$CONFIG_FILE" ]] || { echo "missing: ${CONFIG_FILE}"; status=1; }
  [[ -f "$SETTINGS_ENV" ]] || { echo "missing: ${SETTINGS_ENV}"; status=1; }
  [[ -f "$ROUTER_ENV" ]] || { echo "missing: ${ROUTER_ENV}"; status=1; }

  if [[ -f "$NODES_STATE_FILE" ]]; then
    python3 "$STATECTL" render-provider "$NODES_STATE_FILE" "$tmpdir/provider.txt"
    if [[ -f "$PROVIDER_FILE" ]] && ! cmp -s "$tmpdir/provider.txt" "$PROVIDER_FILE"; then
      echo "drift: provider file differs from nodes state"
      status=1
    fi
  else
    echo "missing: ${NODES_STATE_FILE}"
    status=1
  fi

  if [[ -f "$RULES_STATE_FILE" ]]; then
    python3 "$STATECTL" render-rules "$RULES_STATE_FILE" "$tmpdir/rules.txt"
    if [[ -f "$RENDERED_RULES_FILE" ]] && ! cmp -s "$tmpdir/rules.txt" "$RENDERED_RULES_FILE"; then
      echo "drift: rendered rules file differs from rules state"
      status=1
    fi
    if ! python3 "$STATECTL" validate-rule-targets "$RULES_STATE_FILE" "$NODES_STATE_FILE" >/tmp/mihomo-audit-targets.log 2>&1; then
      echo "invalid: rule targets reference unavailable targets"
      sed -n '1,20p' /tmp/mihomo-audit-targets.log
      status=1
    fi
  else
    echo "missing: ${RULES_STATE_FILE}"
    status=1
  fi

  if [[ "${ALPHA_AUTO_UPDATE:-0}" == "1" ]]; then
    if ! systemctl_cmd is-enabled mihomo-alpha-update.timer >/dev/null 2>&1; then
      echo "drift: alpha auto-update enabled in settings but timer not enabled"
      status=1
    fi
  fi

  if [[ "${RESTART_INTERVAL_HOURS:-0}" =~ ^[0-9]+$ ]] && [[ "${RESTART_INTERVAL_HOURS:-0}" -gt 0 ]]; then
    if ! systemctl_cmd is-enabled mihomo-restart.timer >/dev/null 2>&1; then
      echo "drift: restart interval configured but restart timer not enabled"
      status=1
    fi
  fi

  if [[ "$status" -eq 0 ]]; then
    ok "安装审计通过"
  fi

  rm -rf "$tmpdir"
  trap - RETURN
  return "$status"
}

sync_rules_repo() {
  require_root
  load_settings_readonly
  ensure_layout

  if [[ "${RULES_AUTO_SYNC:-1}" != "1" ]]; then
    info "规则仓库自动同步已关闭"
    return 0
  fi

  local repo_dir="${RULES_REPO_DIR:-/root/mihomo-rules}"
  local branch
  local export_dir

  if [[ ! -d "${repo_dir}/.git" ]]; then
    warn "规则仓库不存在或不是 Git 仓库: ${repo_dir}"
    return 0
  fi

  export_dir="${repo_dir}/manager/custom-rules"
  mkdir -p "$export_dir"
  cp -f "$RULES_STATE_FILE" "${export_dir}/rules.json"
  cp -f "$RENDERED_RULES_FILE" "${export_dir}/custom.rules"
  {
    echo "# generated by mihomo manager"
    echo "generated_at=$(date '+%F %T %Z')"
    echo "source_host=$(hostname)"
    echo "allowed_builtin_targets=DIRECT,PROXY,REJECT,AUTO"
  } > "${export_dir}/README.txt"

  git_cmd -C "$repo_dir" add manager/custom-rules
  if git_cmd -C "$repo_dir" diff --cached --quiet; then
    info "规则仓库没有变更，无需同步"
    return 0
  fi

  git_cmd -C "$repo_dir" commit -m "chore: sync mihomo custom rules $(date +%F)"
  branch="$(git_cmd -C "$repo_dir" branch --show-current)"
  [[ -n "$branch" ]] || branch="main"
  git_cmd -C "$repo_dir" push origin "$branch"
  ok "已同步自定义规则到规则仓库"
}

sync_rules_repo_if_enabled() {
  load_settings_readonly
  if [[ "${RULES_AUTO_SYNC:-1}" == "1" ]]; then
    sync_rules_repo
  fi
}

runtime_audit() {
  require_root
  load_settings_readonly
  load_router_env_readonly

  local active_state enabled_state sub_state main_pid active_since n_restarts memory_current memory_peak cpu_nsec
  local warn_count err_count
  local trigger_update="disabled"
  local trigger_restart="disabled"

  active_state="$(systemctl_show_value mihomo ActiveState)"
  enabled_state="$(systemctl_cmd is-enabled mihomo 2>/dev/null || true)"
  sub_state="$(systemctl_show_value mihomo SubState)"
  main_pid="$(systemctl_show_value mihomo MainPID)"
  active_since="$(systemctl_show_value mihomo ActiveEnterTimestamp)"
  n_restarts="$(systemctl_show_value mihomo NRestarts)"
  memory_current="$(systemctl_show_value mihomo MemoryCurrent)"
  memory_peak="$(systemctl_show_value mihomo MemoryPeak)"
  cpu_nsec="$(systemctl_show_value mihomo CPUUsageNSec)"

  if systemctl_cmd is-enabled mihomo-alpha-update.timer >/dev/null 2>&1; then
    trigger_update="$(systemctl_show_value mihomo-alpha-update.timer NextElapseUSecRealtime)"
  fi
  if systemctl_cmd is-enabled mihomo-restart.timer >/dev/null 2>&1; then
    trigger_restart="$(systemctl_show_value mihomo-restart.timer NextElapseUSecRealtime)"
  fi

  warn_count="$(journalctl_cmd -u mihomo --since '24 hours ago' -p warning --no-pager 2>/dev/null | grep -c '^' || true)"
  err_count="$(journalctl_cmd -u mihomo --since '24 hours ago' -p err --no-pager 2>/dev/null | grep -c '^' || true)"

  echo "== 运行审计 =="
  echo "服务状态: ${active_state:-unknown}"
  echo "运行子状态: ${sub_state:-unknown}"
  echo "开机自启: ${enabled_state:-unknown}"
  echo "主进程 PID: ${main_pid:-0}"
  echo "本次启动时间: ${active_since:-unknown}"
  echo "自 systemd 接管后的重启次数: ${n_restarts:-0}"
  echo "当前内存占用(字节): ${memory_current:-0}"
  echo "历史峰值内存(字节): ${memory_peak:-0}"
  echo "累计 CPU 时间(ns): ${cpu_nsec:-0}"
  echo "端口监听: mixed=${MIXED_PORT} tproxy=${TPROXY_PORT} dns=${DNS_PORT} controller=${CONTROLLER_PORT}"
  echo "节点统计: 启用=$(readonly_node_counts | cut -f1) 总计=$(readonly_node_counts | cut -f2)"
  echo "过去 24 小时 warning 数: ${warn_count:-0}"
  echo "过去 24 小时 error 数: ${err_count:-0}"
  echo "下次 Alpha 自动更新: ${trigger_update:-disabled}"
  echo "下次定时重启: ${trigger_restart:-disabled}"
  echo
  echo "== 健康摘要 =="
  healthcheck || true
}
