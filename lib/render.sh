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
  chmod 750 "$MIHOMO_DIR" "$RULES_DIR" "$PROVIDER_DIR" "$UI_DIR" "$STATE_DIR" "$SNAPSHOT_DIR"
  [[ -f "$CONFIG_FILE" ]] && chmod 640 "$CONFIG_FILE"
  [[ -f "$PROVIDER_FILE" ]] && chmod 640 "$PROVIDER_FILE"
  [[ -f "$RENDERED_RULES_FILE" ]] && chmod 640 "$RENDERED_RULES_FILE"
  [[ -f "$ACL_RENDERED_RULES_FILE" ]] && chmod 640 "$ACL_RENDERED_RULES_FILE"
  [[ -f "$RULESET_PRESET_RENDERED_FILE" ]] && chmod 640 "$RULESET_PRESET_RENDERED_FILE"
  [[ -f "$NODES_STATE_FILE" ]] && chmod 640 "$NODES_STATE_FILE"
  [[ -f "$RULES_STATE_FILE" ]] && chmod 640 "$RULES_STATE_FILE"
  [[ -f "$ACL_STATE_FILE" ]] && chmod 640 "$ACL_STATE_FILE"
  [[ -f "$SUBSCRIPTIONS_STATE_FILE" ]] && chmod 640 "$SUBSCRIPTIONS_STATE_FILE"
  [[ -f "$SETTINGS_ENV" ]] && chown root:"$MIHOMO_USER" "$SETTINGS_ENV"
  [[ -f "$ROUTER_ENV" ]] && chown root:"$MIHOMO_USER" "$ROUTER_ENV"
  [[ -f "$COUNTRY_MMDB" ]] && chmod 644 "$COUNTRY_MMDB"
  [[ -f "${MIHOMO_DIR}/cache.db" ]] && chmod 640 "${MIHOMO_DIR}/cache.db"
  return 0
}

render_provider_file() {
  require_statectl
  python3 "$STATECTL" render-provider "$NODES_STATE_FILE" "$PROVIDER_FILE" --exclude-source-kind subscription
}

render_rules_file() {
  require_statectl
  python3 "$STATECTL" render-rules "$RULES_STATE_FILE" "$RENDERED_RULES_FILE"
}

render_acl_file() {
  require_statectl
  python3 "$STATECTL" render-rules "$ACL_STATE_FILE" "$ACL_RENDERED_RULES_FILE"
}

render_rule_preset_file() {
  local preset_name="${RULESET_PRESET:-$(default_rule_preset)}"
  require_rulepresetctl
  local manifest_path
  manifest_path="$(rule_preset_manifest_path "$preset_name" 2>/dev/null || true)"
  [[ -n "$manifest_path" && -f "$manifest_path" ]] || die "未找到规则预设: ${preset_name}"
  python3 "$RULEPRESETCTL" render "$manifest_path" "$RULESET_PRESET_RENDERED_FILE"
}

validate_rule_targets_file() {
  local state_file="$1"
  local label="$2"
  require_statectl
  local output
  if ! output="$(python3 "$STATECTL" validate-rule-targets "$state_file" "$NODES_STATE_FILE" 2>&1)"; then
    [[ -n "$output" ]] && printf '%s\n' "$output" >&2
    die "${label}存在指向不存在或未启用节点的目标"
  fi
}

validate_rule_targets() {
  validate_rule_targets_file "$RULES_STATE_FILE" "自定义规则"
  validate_rule_targets_file "$ACL_STATE_FILE" "ACL 规则"
}

render_rules_block() {
  local rule_file="$1"
  if [[ -f "$rule_file" ]]; then
    awk 'NF && $0 !~ /^[[:space:]]*#/' "$rule_file"
  fi
  return 0
}

render_config() {
  require_root
  ensure_layout
  ensure_permissions
  load_settings
  load_router_env
  rule_preset_exists "${RULESET_PRESET:-$(default_rule_preset)}" || die "未知规则预设: ${RULESET_PRESET}"
  validate_rule_targets
  render_provider_file
  render_rules_file
  render_acl_file
  render_rule_preset_file

  local secret
  local manual_enabled_count
  local active_provider_count=0
  local lan_cidrs
  local config_mode
  local allowed_cidr
  local denied_cidr
  local auth_entry
  local skip_auth_prefix
  local enable_ipv6
  local explicit_proxy_only=0
  local sub_idx
  local sub_id
  local sub_name
  local sub_url
  local sub_enabled
  local sub_last_success
  local sub_imported_count
  local sub_last_error
  local provider_name
  local provider_relpath
  local -a lan_allowed_cidrs_arr=()
  local -a active_provider_names=()
  local -a active_subscription_ids=()

  secret="$(current_secret)"
  [[ -n "$secret" ]] || secret="$(random_secret)"
  manual_enabled_count="$(python3 "$STATECTL" enabled-count "$NODES_STATE_FILE" --exclude-source-kind subscription)"
  config_mode="${CONFIG_MODE:-rule}"
  lan_cidrs="${LAN_CIDRS:-192.168.2.0/24}"
  enable_ipv6="${ENABLE_IPV6:-0}"
  [[ "${TEMPLATE_NAME:-}" == "nas-explicit-proxy-only" ]] && explicit_proxy_only=1
  read -r -a lan_allowed_cidrs_arr <<< "${lan_cidrs}"
  lan_allowed_cidrs_arr+=("127.0.0.0/8")
  if [[ "$manual_enabled_count" -gt 0 ]]; then
    active_provider_names+=("manual")
    active_provider_count=$((active_provider_count + 1))
  fi
  while IFS=$'\t' read -r sub_idx sub_id sub_name sub_url sub_enabled sub_last_success sub_imported_count sub_last_error; do
    [[ "$sub_enabled" == "1" ]] || continue
    [[ -s "$(subscription_provider_file "$sub_id")" ]] || continue
    active_provider_names+=("$(subscription_provider_name "$sub_id")")
    active_subscription_ids+=("$sub_id")
    active_provider_count=$((active_provider_count + 1))
  done < <(subscription_list_tsv || true)

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
  if [[ ${#LAN_DISALLOWED_CIDRS_ARR[@]} -gt 0 ]]; then
    cat >>"$CONFIG_FILE" <<'EOF'
lan-disallowed-ips:
EOF
    for denied_cidr in "${LAN_DISALLOWED_CIDRS_ARR[@]}"; do
      [[ -n "$denied_cidr" ]] || continue
      printf '  - %s\n' "$denied_cidr" >>"$CONFIG_FILE"
    done
  fi
  cat >>"$CONFIG_FILE" <<EOF
mode: ${config_mode}
log-level: info
ipv6: $([[ "$enable_ipv6" == "1" ]] && echo true || echo false)
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
  ipv6: $([[ "$enable_ipv6" == "1" ]] && echo true || echo false)
  use-hosts: true
  use-system-hosts: true
  cache-algorithm: arc
  respect-rules: false
  prefer-h3: false
  enhanced-mode: fake-ip
  fake-ip-range: 198.18.0.1/16
  fake-ip-filter-mode: blacklist
  fake-ip-filter:
    - "*.lan"
    - "*.local"
    - "+.arpa"
    - "+.stun.*.*"
    - "localhost.ptlogin2.qq.com"
    - "+.msftconnecttest.com"
    - "+.msftncsi.com"
    - "captive.apple.com"
    - "connectivitycheck.gstatic.com"
  default-nameserver:
    - 223.5.5.5
    - 119.29.29.29
  nameserver-policy:
    "geosite:private,cn":
      - 223.5.5.5
      - 119.29.29.29
      - https://dns.alidns.com/dns-query
      - https://doh.pub/dns-query
    "+.arpa":
      - 223.5.5.5
      - 119.29.29.29
      - https://dns.alidns.com/dns-query
      - https://doh.pub/dns-query
  nameserver:
    - https://cloudflare-dns.com/dns-query#RULES
    - https://dns.google/dns-query#RULES
  fallback: []
  fallback-filter:
    geoip: false
  direct-nameserver:
    - https://dns.alidns.com/dns-query
    - https://doh.pub/dns-query
  direct-nameserver-follow-policy: true
  proxy-server-nameserver:
    - 223.5.5.5
    - 119.29.29.29
proxies: []
EOF

  if [[ ${#PROXY_AUTH_CREDENTIALS_ARR[@]} -gt 0 ]]; then
    cat >>"$CONFIG_FILE" <<'EOF'
authentication:
EOF
    for auth_entry in "${PROXY_AUTH_CREDENTIALS_ARR[@]}"; do
      [[ -n "$auth_entry" ]] || continue
      printf '  - %s\n' "$(printf '%s' "$auth_entry" | python3 -c 'import json,sys; print(json.dumps(sys.stdin.read()).strip())')" >>"$CONFIG_FILE"
    done
    if [[ ${#SKIP_AUTH_PREFIXES_ARR[@]} -gt 0 ]]; then
      cat >>"$CONFIG_FILE" <<'EOF'
skip-auth-prefixes:
EOF
      for skip_auth_prefix in "${SKIP_AUTH_PREFIXES_ARR[@]}"; do
        [[ -n "$skip_auth_prefix" ]] || continue
        printf '  - %s\n' "$skip_auth_prefix" >>"$CONFIG_FILE"
      done
    fi
  fi

  if [[ "$active_provider_count" -gt 0 ]]; then
    cat >>"$CONFIG_FILE" <<'EOF'

proxy-providers:
EOF
    if [[ "$manual_enabled_count" -gt 0 ]]; then
      cat >>"$CONFIG_FILE" <<'EOF'
  manual:
    type: file
    path: ./proxy_providers/manual.txt
    health-check:
      enable: true
      url: "https://cp.cloudflare.com/generate_204"
      interval: 300
      timeout: 5000
      lazy: true
EOF
    fi
    for sub_id in "${active_subscription_ids[@]}"; do
      provider_name="$(subscription_provider_name "$sub_id")"
      provider_relpath="$(subscription_provider_relpath "$sub_id")"
      cat >>"$CONFIG_FILE" <<EOF
  ${provider_name}:
    type: file
    path: ${provider_relpath}
    health-check:
      enable: true
      url: "https://cp.cloudflare.com/generate_204"
      interval: 300
      timeout: 5000
      lazy: true
EOF
    done
    cat >>"$CONFIG_FILE" <<'EOF'

proxy-groups:
  - name: "PROXY"
    type: select
    proxies:
      - DIRECT
      - AUTO
    use:
EOF
    for provider_name in "${active_provider_names[@]}"; do
      printf '      - %s\n' "$provider_name" >>"$CONFIG_FILE"
    done
    cat >>"$CONFIG_FILE" <<'EOF'

  - name: "AUTO"
    type: url-test
    url: "https://cp.cloudflare.com/generate_204"
    interval: 300
    tolerance: 80
    lazy: true
    use:
EOF
    for provider_name in "${active_provider_names[@]}"; do
      printf '      - %s\n' "$provider_name" >>"$CONFIG_FILE"
    done
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
  done < <(render_rules_block "$RENDERED_RULES_FILE")
  while IFS= read -r custom_rule; do
    [[ -n "$custom_rule" ]] || continue
    printf '  - %s\n' "$custom_rule" >>"$CONFIG_FILE"
  done < <(render_rules_block "$ACL_RENDERED_RULES_FILE")
  while IFS= read -r custom_rule; do
    [[ -n "$custom_rule" ]] || continue
    printf '  - %s\n' "$custom_rule" >>"$CONFIG_FILE"
  done < <(render_rules_block "$RULESET_PRESET_RENDERED_FILE")

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
  load_router_env
  cat >"$ROUTER_SYSCTL" <<EOF
net.ipv4.ip_forward = 1
net.ipv4.conf.all.route_localnet = 1
net.ipv4.conf.default.rp_filter = 2
net.ipv4.conf.all.rp_filter = 2
$( [[ "${ENABLE_IPV6:-0}" == "1" ]] && printf '%s\n' 'net.ipv6.conf.all.forwarding = 1' || true )
EOF
  sysctl -p "$ROUTER_SYSCTL" >/dev/null
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

geosite_probe_file() {
  local geosite_file="$1"
  [[ -x "$MIHOMO_BIN" ]] || return 1
  [[ -f "$geosite_file" ]] || return 1

  local tmpdir
  tmpdir="$(mktemp -d)"
  trap "rm -rf '$tmpdir'" RETURN

  cp -a "$geosite_file" "${tmpdir}/GeoSite.dat"
  cat > "${tmpdir}/config.yaml" <<'EOF'
mode: rule
log-level: info
geodata-mode: false
geo-auto-update: false
dns:
  enable: true
  listen: 127.0.0.1:1053
  ipv6: false
  enhanced-mode: fake-ip
  fake-ip-range: 198.18.0.1/16
  nameserver-policy:
    "geosite:cn":
      - 223.5.5.5
  nameserver:
    - 223.5.5.5
proxies: []
proxy-groups:
  - name: "PROXY"
    type: select
    proxies:
      - DIRECT
rules:
  - MATCH,DIRECT
EOF

  timeout 8 "$MIHOMO_BIN" -t -d "$tmpdir" >/tmp/mihomo-geosite-probe.out 2>/tmp/mihomo-geosite-probe.err
}

download_geosite_to_temp() {
  local target_file="$1"
  local url
  local urls=(
    "https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/geosite.dat"
    "https://cdn.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@release/geosite.dat"
    "https://testingcf.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@release/geosite.dat"
  )

  rm -f "$target_file"
  for url in "${urls[@]}"; do
    info "下载 GeoSite.dat: ${url}"
    if curl_cmd -fL --progress-bar --retry 2 --retry-delay 2 --connect-timeout 10 --max-time 300 -o "$target_file" "$url"; then
      return 0
    fi
    warn "GeoSite.dat 下载失败，尝试下一个源: ${url}"
    rm -f "$target_file"
  done
  return 1
}

install_geosite_dat() {
  require_root
  local tmp_geosite
  tmp_geosite="$(mktemp)"
  rm -f "$tmp_geosite"

  download_geosite_to_temp "$tmp_geosite" || {
    rm -f "$tmp_geosite"
    die "GeoSite.dat 所有下载源都失败；请检查当前网络或稍后重试"
  }

  geosite_probe_file "$tmp_geosite" || {
    rm -f "$tmp_geosite"
    warn "GeoSite.dat 验证失败:"
    sed -n '1,40p' /tmp/mihomo-geosite-probe.out 2>/dev/null || true
    sed -n '1,40p' /tmp/mihomo-geosite-probe.err 2>/dev/null || true
    die "GeoSite.dat 未通过验证；已停止安装，避免把坏资产写进运行目录"
  }

  install -m 0644 "$tmp_geosite" "${MIHOMO_DIR}/GeoSite.dat"
  chown "${MIHOMO_USER}:${MIHOMO_USER}" "${MIHOMO_DIR}/GeoSite.dat"
  rm -f "$tmp_geosite"
  ok "GeoSite.dat 已更新并通过验证"
}

ensure_geosite_ready() {
  if geosite_probe_ready; then
    return 0
  fi
  warn "GeoSite.dat 缺失或不可用，开始自动修复"
  install_geosite_dat
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
  if ! curl_cmd -fL --progress-bar -o "${tmp}/ui.zip" "$ui_url"; then
    warn "WebUI 下载失败: ${ui_name}"
    rm -rf "$tmp"
    trap - RETURN
    return 1
  fi
  if ! unzip -q "${tmp}/ui.zip" -d "$tmp" >/dev/null 2>&1; then
    warn "WebUI 解压失败: ${tmp}/ui.zip"
    rm -rf "$tmp"
    trap - RETURN
    return 1
  fi
  src="$(find "$tmp" -maxdepth 1 -mindepth 1 -type d | head -n 1)"
  if [[ -z "$src" ]]; then
    warn "未找到解压后的 WebUI 目录"
    rm -rf "$tmp"
    trap - RETURN
    return 1
  fi
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
  rm -rf "$INSTALL_ROOT"
  mkdir -p "$(dirname "$INSTALL_ROOT")"
  cp -a "$src_root" "$INSTALL_ROOT"
  rm -rf "$INSTALL_ROOT/.git" "$INSTALL_ROOT/.codex"
  find "$INSTALL_ROOT" -type d -name '__pycache__' -prune -exec rm -rf {} +
  find "$INSTALL_ROOT" -type f \( -name '*.bak.*' -o -name '*.pyc' \) -delete
  chmod +x "$INSTALL_ROOT/mihomo" "$INSTALL_ROOT/scripts/statectl.py"
  ln -sf "$INSTALL_ROOT/mihomo" "$MANAGER_BIN"
  ln -sf "$INSTALL_ROOT/mihomo" "$COMPAT_MANAGER_BIN"
  ok "已安装管理命令到 ${MANAGER_BIN}"
}

write_manager_sync_units() {
  local src_root="$1"
  local interval_minutes="$2"
  cat >"$MANAGER_SYNC_SERVICE_UNIT" <<EOF
[Unit]
Description=Sync Mihomo Manager From Working Tree
ConditionPathExists=${src_root}/.git
ConditionPathExists=${src_root}/mihomo

[Service]
Type=oneshot
WorkingDirectory=${src_root}
ExecStart=${src_root}/mihomo install-self
EOF
  cat >"$MANAGER_SYNC_TIMER_UNIT" <<EOF
[Unit]
Description=Periodic Mihomo Manager Working Tree Sync Timer

[Timer]
OnBootSec=1min
OnUnitActiveSec=${interval_minutes}min
AccuracySec=15s
Persistent=true
Unit=mihomo-manager-sync.service

[Install]
WantedBy=timers.target
EOF
}

install_project_sync() {
  require_root
  local src_root="$1"
  local interval_minutes="${2:-1}"
  [[ -d "${src_root}/.git" ]] || die "install-self-sync 只能从 git 工作树执行"
  [[ -x "${src_root}/mihomo" ]] || die "未找到源码入口: ${src_root}/mihomo"
  [[ "$interval_minutes" =~ ^[0-9]+$ ]] || die "同步间隔必须是正整数分钟"
  [[ "$interval_minutes" -gt 0 ]] || die "同步间隔必须大于 0 分钟"

  ensure_settings
  install_project "$src_root"
  write_manager_sync_units "$src_root" "$interval_minutes"
  upsert_env_var "$SETTINGS_ENV" "MANAGER_SYNC_ENABLED" "1"
  upsert_env_var "$SETTINGS_ENV" "MANAGER_SYNC_INTERVAL_MINUTES" "$interval_minutes"
  upsert_env_var "$SETTINGS_ENV" "MANAGER_SYNC_SOURCE" "$src_root"
  systemctl_cmd daemon-reload
  systemctl_cmd enable --now mihomo-manager-sync.timer
  ok "已启用本机源码自动同步: 每 ${interval_minutes} 分钟从 ${src_root} 同步到 ${INSTALL_ROOT}"
}

disable_project_sync() {
  require_root
  ensure_settings
  upsert_env_var "$SETTINGS_ENV" "MANAGER_SYNC_ENABLED" "0"
  upsert_env_var "$SETTINGS_ENV" "MANAGER_SYNC_INTERVAL_MINUTES" "1"
  upsert_env_var "$SETTINGS_ENV" "MANAGER_SYNC_SOURCE" ""
  systemctl_cmd disable --now mihomo-manager-sync.timer >/dev/null 2>&1 || true
  rm -f "$MANAGER_SYNC_SERVICE_UNIT" "$MANAGER_SYNC_TIMER_UNIT"
  systemctl_cmd daemon-reload
  ok "已关闭本机源码自动同步"
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
  ensure_enabled_nodes
  if [[ ${#PROXY_INGRESS_IFACES_ARR[@]} -eq 0 && "${DNS_HIJACK_ENABLED}" != "1" && "${PROXY_HOST_OUTPUT}" != "1" ]]; then
    clear_rules
    ok "当前模板为仅显式代理，不下发透明旁路由规则"
    return 0
  fi
  require_firewall_support
  resolve_bypass_container_ips
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

iptables_counter_sum() {
  local table="$1"
  local chain="$2"
  local target="$3"
  iptables_cmd -t "$table" -L "$chain" -v -n -x 2>/dev/null | awk -v target="$target" '$3 == target {sum += $1} END {print sum + 0}'
}

localhost_proxy_probe() {
  curl_cmd --noproxy '*' -fsS --max-time 10 -x "http://127.0.0.1:${MIXED_PORT}" https://cp.cloudflare.com/generate_204 >/tmp/mihomo-health-proxy.out 2>/dev/null
}

local_controller_probe() {
  curl_cmd --noproxy '*' -fsS --max-time 10 "http://127.0.0.1:${CONTROLLER_PORT}/ui/" >/tmp/mihomo-health-ui.html 2>/dev/null
}

listener_snapshot() {
  ss_cmd -lntup 2>/dev/null || true
}

listener_has_port() {
  local listeners="$1"
  local port="$2"
  grep -qE "[:.]${port}[[:space:]]" <<<"$listeners"
}

healthcheck() {
  require_root
  load_router_env
  local failed=0
  local listeners
  listeners="$(listener_snapshot)"
  service_is_active || { echo "service: inactive"; failed=1; }
  [[ -f "$COUNTRY_MMDB" ]] || { echo "geo: missing Country.mmdb"; failed=1; }
  listener_has_port "$listeners" "$MIXED_PORT" || { echo "port: mixed ${MIXED_PORT} not listening"; failed=1; }
  listener_has_port "$listeners" "$TPROXY_PORT" || { echo "port: tproxy ${TPROXY_PORT} not listening"; failed=1; }
  listener_has_port "$listeners" "$DNS_PORT" || { echo "port: dns ${DNS_PORT} not listening"; failed=1; }
  listener_has_port "$listeners" "$CONTROLLER_PORT" || { echo "port: controller ${CONTROLLER_PORT} not listening"; failed=1; }
  local_controller_probe || {
    echo "webui: unavailable"
    failed=1
  }
  localhost_proxy_probe || {
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

geosite_probe_ready() {
  geosite_probe_file "${MIHOMO_DIR}/GeoSite.dat"
}

audit_installation() {
  require_root
  load_settings_readonly
  load_router_env_readonly
  local status=0
  local tmpdir
  local manifest_path
  local rule_preset_name="${RULESET_PRESET:-$(default_rule_preset)}"
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

  if [[ -f "$ACL_STATE_FILE" ]]; then
    python3 "$STATECTL" render-rules "$ACL_STATE_FILE" "$tmpdir/acl.txt"
    if [[ -f "$ACL_RENDERED_RULES_FILE" ]] && ! cmp -s "$tmpdir/acl.txt" "$ACL_RENDERED_RULES_FILE"; then
      echo "drift: rendered acl file differs from acl state"
      status=1
    fi
    if ! python3 "$STATECTL" validate-rule-targets "$ACL_STATE_FILE" "$NODES_STATE_FILE" >/tmp/mihomo-audit-acl-targets.log 2>&1; then
      echo "invalid: acl targets reference unavailable targets"
      sed -n '1,20p' /tmp/mihomo-audit-acl-targets.log
      status=1
    fi
  else
    echo "missing: ${ACL_STATE_FILE}"
    status=1
  fi

  if ! rule_preset_exists "$rule_preset_name"; then
    echo "invalid: unknown rule preset ${rule_preset_name}"
    status=1
  else
    require_rulepresetctl
    manifest_path="$(rule_preset_manifest_path "$rule_preset_name")"
    python3 "$RULEPRESETCTL" render "$manifest_path" "$tmpdir/builtin.rules"
    if [[ ! -f "$RULESET_PRESET_RENDERED_FILE" ]]; then
      echo "missing: ${RULESET_PRESET_RENDERED_FILE}"
      status=1
    elif ! cmp -s "$tmpdir/builtin.rules" "$RULESET_PRESET_RENDERED_FILE"; then
      echo "drift: rendered builtin rules differ from rule preset"
      status=1
    fi
  fi

  [[ -f "$SUBSCRIPTIONS_STATE_FILE" ]] || { echo "missing: ${SUBSCRIPTIONS_STATE_FILE}"; status=1; }

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

  if [[ -f "${MIHOMO_DIR}/GeoSite.dat" ]]; then
    if geosite_probe_ready; then
      echo "ok: GeoSite.dat 可用于 geosite 规则"
    else
      echo "invalid: GeoSite.dat 当前不可用于 geosite 规则"
      sed -n '1,20p' /tmp/mihomo-geosite-probe.out 2>/dev/null || true
      sed -n '1,20p' /tmp/mihomo-geosite-probe.err 2>/dev/null || true
      status=1
    fi
  else
    echo "missing: ${MIHOMO_DIR}/GeoSite.dat"
    status=1
  fi

  if [[ "$status" -eq 0 ]]; then
    ok "安装审计通过"
  fi

  rm -rf "$tmpdir"
  trap - RETURN
  return "$status"
}

runtime_audit() {
  require_root
  load_settings_readonly
  load_router_env_readonly

  local active_state enabled_state sub_state main_pid active_since n_restarts memory_current memory_peak cpu_nsec
  local warn_count err_count
  local trigger_update="disabled"
  local trigger_restart="disabled"
  local controller_scope controller_host proxy_probe="failed" controller_probe="failed"
  local tproxy_packets dns_hijack_packets lan_activity_summary

  controller_scope_summary
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
  if localhost_proxy_probe; then
    proxy_probe="ok"
  fi
  if local_controller_probe; then
    controller_probe="ok"
  fi
  tproxy_packets="$(iptables_counter_sum mangle MIHOMO_PRE_HANDLE TPROXY)"
  dns_hijack_packets="$(iptables_counter_sum nat MIHOMO_DNS_HANDLE REDIRECT)"
  if [[ "$tproxy_packets" -gt 0 || "$dns_hijack_packets" -gt 0 ]]; then
    lan_activity_summary="近期已观测到局域网旁路由流量"
  else
    lan_activity_summary="当前未观测到局域网旁路由命中包；若你刚切好网关/DNS，可再从局域网设备发起一次请求"
  fi

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
  echo "当前模板: ${TEMPLATE_NAME:-unknown} ($(template_summary "${TEMPLATE_NAME:-unknown}"))"
  echo "规则预设: ${RULESET_PRESET:-$(default_rule_preset)} ($(rule_preset_summary "${RULESET_PRESET:-$(default_rule_preset)}"))"
  echo "IPv6 模式: $([[ "${ENABLE_IPV6:-0}" == "1" ]] && echo '启用' || echo '关闭')"
  echo "控制面范围: ${CONTROLLER_SCOPE}"
  echo "局域网旁路由入口: ${PROXY_INGRESS_INTERFACES:-未配置}"
  echo "局域网网段: ${LAN_CIDRS:-未设置}"
  echo "局域网禁止网段: ${LAN_DISALLOWED_CIDRS:-无}"
  echo "DNS 劫持入口: $([[ "${DNS_HIJACK_ENABLED}" == "1" ]] && echo "${DNS_HIJACK_INTERFACES:-未配置}" || echo '关闭')"
  echo "宿主机流量模式: $([[ "${PROXY_HOST_OUTPUT}" == "1" ]] && echo '透明接管(高风险)' || echo '默认直连 + localhost 显式代理')"
  echo "显式代理认证: $([[ -n "${PROXY_AUTH_CREDENTIALS:-}" ]] && echo "启用 (${#PROXY_AUTH_CREDENTIALS_ARR[@]} 组账号)" || echo '关闭')"
  echo "显式代理免认证网段: $([[ -n "${SKIP_AUTH_PREFIXES:-}" ]] && echo "${SKIP_AUTH_PREFIXES}" || echo '无')"
  echo "localhost 显式代理探测: ${proxy_probe}"
  echo "本机 WebUI 探测: ${controller_probe}"
  echo "局域网透明代理命中包数: ${tproxy_packets}"
  echo "DNS 劫持命中包数: ${dns_hijack_packets}"
  echo "旁路由流量摘要: ${lan_activity_summary}"
  echo "节点统计: 启用=$(readonly_node_counts | cut -f1) 总计=$(readonly_node_counts | cut -f2)"
  echo "过去 24 小时 warning 数: ${warn_count:-0}"
  echo "过去 24 小时 error 数: ${err_count:-0}"
  echo "下次 Alpha 自动更新: ${trigger_update:-disabled}"
  echo "下次定时重启: ${trigger_restart:-disabled}"
  echo
  echo "== 健康摘要 =="
  healthcheck || true
}
