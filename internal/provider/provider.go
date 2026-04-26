package provider

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	yaml "gopkg.in/yaml.v2"

	"minimalist/internal/state"
)

type ScanRow struct {
	URI       string
	Name      string
	Server    string
	Port      string
	Network   string
	Security  string
	Supported string
	Scheme    string
	Reason    string
}

type uriInfo struct {
	Scheme          string
	Server          string
	Port            int
	UUID            string
	Password        string
	Cipher          string
	Network         string
	Flow            string
	PacketEncoding  string
	ALPN            []string
	ServerName      string
	Fingerprint     string
	Encryption      string
	SkipCertVerify  *bool
	RealityOpts     map[string]any
	Path            string
	Host            string
	Mode            string
	ServiceName     string
	DownloadSetting map[string]any
	Plugin          string
	PluginOpts      map[string]any
	AlterID         int
	TLS             bool
	HeaderType      string
}

type vlessProvider struct {
	Name            string         `yaml:"name"`
	Type            string         `yaml:"type"`
	Server          string         `yaml:"server"`
	Port            int            `yaml:"port"`
	UUID            string         `yaml:"uuid"`
	UDP             bool           `yaml:"udp"`
	Flow            string         `yaml:"flow,omitempty"`
	PacketEncoding  string         `yaml:"packet-encoding,omitempty"`
	Encryption      string         `yaml:"encryption,omitempty"`
	TLS             bool           `yaml:"tls,omitempty"`
	ALPN            []string       `yaml:"alpn,omitempty"`
	ServerName      string         `yaml:"servername,omitempty"`
	Fingerprint     string         `yaml:"client-fingerprint,omitempty"`
	SkipCertVerify  *bool          `yaml:"skip-cert-verify,omitempty"`
	RealityOpts     map[string]any `yaml:"reality-opts,omitempty"`
	Network         string         `yaml:"network"`
	WSOpts          map[string]any `yaml:"ws-opts,omitempty"`
	GRPCOpts        map[string]any `yaml:"grpc-opts,omitempty"`
	HTTPUpgradeOpts map[string]any `yaml:"http-upgrade-opts,omitempty"`
	H2Opts          map[string]any `yaml:"h2-opts,omitempty"`
	Header          map[string]any `yaml:"header,omitempty"`
	XHTTPOpts       map[string]any `yaml:"xhttp-opts,omitempty"`
}

type trojanProvider struct {
	Name            string         `yaml:"name"`
	Type            string         `yaml:"type"`
	Server          string         `yaml:"server"`
	Port            int            `yaml:"port"`
	Password        string         `yaml:"password"`
	UDP             bool           `yaml:"udp"`
	TLS             bool           `yaml:"tls,omitempty"`
	ALPN            []string       `yaml:"alpn,omitempty"`
	ServerName      string         `yaml:"servername,omitempty"`
	Fingerprint     string         `yaml:"client-fingerprint,omitempty"`
	SkipCertVerify  *bool          `yaml:"skip-cert-verify,omitempty"`
	Network         string         `yaml:"network"`
	WSOpts          map[string]any `yaml:"ws-opts,omitempty"`
	GRPCOpts        map[string]any `yaml:"grpc-opts,omitempty"`
	HTTPUpgradeOpts map[string]any `yaml:"http-upgrade-opts,omitempty"`
	H2Opts          map[string]any `yaml:"h2-opts,omitempty"`
	Header          map[string]any `yaml:"header,omitempty"`
}

type ssProvider struct {
	Name       string         `yaml:"name"`
	Type       string         `yaml:"type"`
	Server     string         `yaml:"server"`
	Port       int            `yaml:"port"`
	Cipher     string         `yaml:"cipher"`
	Password   string         `yaml:"password"`
	UDP        bool           `yaml:"udp"`
	Plugin     string         `yaml:"plugin,omitempty"`
	PluginOpts map[string]any `yaml:"plugin-opts,omitempty"`
}

type vmessProvider struct {
	Name            string         `yaml:"name"`
	Type            string         `yaml:"type"`
	Server          string         `yaml:"server"`
	Port            int            `yaml:"port"`
	UUID            string         `yaml:"uuid"`
	AlterID         int            `yaml:"alterId"`
	Cipher          string         `yaml:"cipher"`
	UDP             bool           `yaml:"udp"`
	TLS             bool           `yaml:"tls,omitempty"`
	ALPN            []string       `yaml:"alpn,omitempty"`
	ServerName      string         `yaml:"servername,omitempty"`
	Fingerprint     string         `yaml:"client-fingerprint,omitempty"`
	SkipCertVerify  *bool          `yaml:"skip-cert-verify,omitempty"`
	Network         string         `yaml:"network"`
	WSOpts          map[string]any `yaml:"ws-opts,omitempty"`
	GRPCOpts        map[string]any `yaml:"grpc-opts,omitempty"`
	HTTPUpgradeOpts map[string]any `yaml:"http-upgrade-opts,omitempty"`
	H2Opts          map[string]any `yaml:"h2-opts,omitempty"`
	Header          map[string]any `yaml:"header,omitempty"`
}

type providerFile struct {
	Proxies []any `yaml:"proxies"`
}

func RenderProvider(path string, nodes []state.Node, sourceKind string, excludeSourceKind string) error {
	proxies := []any{}
	for _, node := range nodes {
		if !node.Enabled {
			continue
		}
		if sourceKind != "" && node.Source.Kind != sourceKind {
			continue
		}
		if excludeSourceKind != "" && node.Source.Kind == excludeSourceKind {
			continue
		}
		item, err := providerItemFromNode(node)
		if err != nil {
			return err
		}
		proxies = append(proxies, item)
	}
	body, err := yaml.Marshal(providerFile{Proxies: proxies})
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, body, 0o640)
}

func DecodeSubscriptionLines(text string) []string {
	lines := []string{}
	raw := []string{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			raw = append(raw, line)
		}
	}
	for _, line := range raw {
		if strings.Contains(line, "://") {
			return raw
		}
	}
	collapsed := strings.Join(raw, "")
	if collapsed == "" {
		return nil
	}
	if decoded, err := b64decodePadded(collapsed); err == nil {
		for _, line := range strings.Split(string(decoded), "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				lines = append(lines, line)
			}
		}
		if len(lines) > 0 {
			return lines
		}
	}
	return raw
}

func ScannableSubscriptionURIs(text string) []string {
	uris := []string{}
	for _, line := range DecodeSubscriptionLines(text) {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "://") {
			uris = append(uris, line)
		}
	}
	return uris
}

func ScanURIRows(text string) []ScanRow {
	rows := []ScanRow{}
	for _, uri := range ScannableSubscriptionURIs(text) {
		rows = append(rows, ScanURIRow(uri))
	}
	return rows
}

func ScanURIRow(uri string) ScanRow {
	info, err := parseURIInfo(uri)
	scheme := uriScheme(uri)
	if err != nil {
		return ScanRow{
			URI:       uri,
			Name:      guessName(uri),
			Server:    splitHost(uri),
			Port:      splitPort(uri),
			Network:   queryField(uri, "type", "tcp"),
			Security:  queryField(uri, "security", scheme),
			Supported: "0",
			Scheme:    scheme,
			Reason:    err.Error(),
		}
	}
	return ScanRow{
		URI:       uri,
		Name:      guessName(uri),
		Server:    info.Server,
		Port:      strconv.Itoa(info.Port),
		Network:   defaultString(info.Network, "tcp"),
		Security:  defaultString(securityName(info), scheme),
		Supported: "1",
		Scheme:    scheme,
		Reason:    "",
	}
}

func GuessName(uri string) string { return guessName(uri) }

func URIBaseKey(uri string) string {
	raw := strings.TrimSpace(uri)
	if strings.HasPrefix(raw, "vmess://") {
		data, err := parseJSONFromVmess(raw)
		if err != nil {
			return raw
		}
		delete(data, "ps")
		body, _ := json.Marshal(data)
		return "vmess://" + base64.StdEncoding.EncodeToString(body)
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	parsed.Fragment = ""
	return parsed.String()
}

func providerItemFromNode(node state.Node) (any, error) {
	info, err := parseURIInfo(node.URI)
	if err != nil {
		return nil, err
	}
	switch info.Scheme {
	case "vless":
		return buildVlessProvider(node.Name, info), nil
	case "trojan":
		return buildTrojanProvider(node.Name, info), nil
	case "ss":
		return buildSSProvider(node.Name, info), nil
	case "vmess":
		return buildVMessProvider(node.Name, info), nil
	default:
		return nil, fmt.Errorf("unsupported scheme: %s", info.Scheme)
	}
}

func buildVlessProvider(name string, info uriInfo) vlessProvider {
	item := vlessProvider{
		Name:           name,
		Type:           "vless",
		Server:         info.Server,
		Port:           info.Port,
		UUID:           info.UUID,
		UDP:            true,
		Flow:           info.Flow,
		PacketEncoding: info.PacketEncoding,
		Encryption:     info.Encryption,
		Network:        defaultString(info.Network, "tcp"),
	}
	applyTLSFields(&item.ALPN, &item.ServerName, &item.Fingerprint, &item.SkipCertVerify, info)
	if securityName(info) == "reality" && len(info.RealityOpts) > 0 {
		item.RealityOpts = info.RealityOpts
		item.TLS = true
	}
	if securityName(info) == "tls" {
		item.TLS = true
	}
	applyNetworkFields(&item.Network, &item.WSOpts, &item.GRPCOpts, &item.HTTPUpgradeOpts, &item.H2Opts, &item.Header, info)
	if item.Network == "xhttp" {
		opts := map[string]any{}
		if info.Path != "" {
			opts["path"] = info.Path
		}
		if info.Host != "" {
			opts["host"] = info.Host
		}
		if info.Mode != "" {
			opts["mode"] = info.Mode
		}
		if len(info.DownloadSetting) > 0 {
			opts["download-settings"] = info.DownloadSetting
		}
		if len(opts) > 0 {
			item.XHTTPOpts = opts
		}
	}
	return item
}

func buildTrojanProvider(name string, info uriInfo) trojanProvider {
	item := trojanProvider{
		Name:     name,
		Type:     "trojan",
		Server:   info.Server,
		Port:     info.Port,
		Password: info.Password,
		UDP:      true,
		Network:  defaultString(info.Network, "tcp"),
	}
	if s := securityName(info); s == "tls" || s == "reality" {
		item.TLS = true
	}
	applyTLSFields(&item.ALPN, &item.ServerName, &item.Fingerprint, &item.SkipCertVerify, info)
	applyNetworkFields(&item.Network, &item.WSOpts, &item.GRPCOpts, &item.HTTPUpgradeOpts, &item.H2Opts, &item.Header, info)
	return item
}

func buildSSProvider(name string, info uriInfo) ssProvider {
	return ssProvider{
		Name:       name,
		Type:       "ss",
		Server:     info.Server,
		Port:       info.Port,
		Cipher:     info.Cipher,
		Password:   info.Password,
		UDP:        true,
		Plugin:     info.Plugin,
		PluginOpts: info.PluginOpts,
	}
}

func buildVMessProvider(name string, info uriInfo) vmessProvider {
	item := vmessProvider{
		Name:    name,
		Type:    "vmess",
		Server:  info.Server,
		Port:    info.Port,
		UUID:    info.UUID,
		AlterID: info.AlterID,
		Cipher:  defaultString(info.Cipher, "auto"),
		UDP:     true,
		Network: defaultString(info.Network, "tcp"),
		TLS:     info.TLS,
	}
	applyTLSFields(&item.ALPN, &item.ServerName, &item.Fingerprint, &item.SkipCertVerify, info)
	applyNetworkFields(&item.Network, &item.WSOpts, &item.GRPCOpts, &item.HTTPUpgradeOpts, &item.H2Opts, &item.Header, info)
	return item
}

func applyTLSFields(alpn *[]string, serverName *string, fingerprint *string, skipCertVerify **bool, info uriInfo) {
	*alpn = info.ALPN
	*serverName = info.ServerName
	*fingerprint = info.Fingerprint
	*skipCertVerify = info.SkipCertVerify
}

func applyNetworkFields(network *string, wsOpts, grpcOpts, httpUpgradeOpts, h2Opts, header *map[string]any, info uriInfo) {
	*network = defaultString(info.Network, "tcp")
	switch *network {
	case "ws":
		opts := map[string]any{}
		if info.Path != "" {
			opts["path"] = info.Path
		}
		if info.Host != "" {
			opts["headers"] = map[string]any{"Host": info.Host}
		}
		if len(opts) > 0 {
			*wsOpts = opts
		}
	case "grpc":
		if info.ServiceName != "" {
			*grpcOpts = map[string]any{"grpc-service-name": info.ServiceName}
		}
	case "httpupgrade":
		opts := map[string]any{}
		if info.Path != "" {
			opts["path"] = info.Path
		}
		if info.Host != "" {
			opts["host"] = info.Host
		}
		if len(opts) > 0 {
			*httpUpgradeOpts = opts
		}
	case "http", "h2":
		opts := map[string]any{}
		if info.Host != "" {
			opts["host"] = []string{info.Host}
		}
		if info.Path != "" {
			opts["path"] = info.Path
		}
		if len(opts) > 0 {
			*h2Opts = opts
		}
	case "tcp":
		if info.HeaderType != "" {
			*header = map[string]any{"type": info.HeaderType}
		}
	}
}

func parseURIInfo(uri string) (uriInfo, error) {
	raw := strings.TrimSpace(uri)
	switch uriScheme(raw) {
	case "vless":
		return parseVless(raw)
	case "trojan":
		return parseTrojan(raw)
	case "ss":
		return parseSS(raw)
	case "vmess":
		return parseVMess(raw)
	default:
		return uriInfo{}, fmt.Errorf("unsupported scheme: %s", uriScheme(raw))
	}
}

func parseVless(raw string) (uriInfo, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return uriInfo{}, err
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil || parsed.Hostname() == "" || parsed.User == nil {
		return uriInfo{}, fmt.Errorf("invalid vless uri")
	}
	query := parsed.Query()
	extra := query.Get("extra")
	download := map[string]any{}
	if extra != "" {
		var payload map[string]any
		if json.Unmarshal([]byte(extra), &payload) == nil {
			if setting, ok := payload["downloadSettings"].(map[string]any); ok {
				download = xhttpDownloadSettings(setting)
			}
			if setting, ok := payload["download-settings"].(map[string]any); ok && len(download) == 0 {
				download = xhttpDownloadSettings(setting)
			}
		}
	}
	info := uriInfo{
		Scheme:          "vless",
		Server:          parsed.Hostname(),
		Port:            port,
		UUID:            parsed.User.Username(),
		Network:         defaultString(strings.ToLower(query.Get("type")), "tcp"),
		Flow:            query.Get("flow"),
		PacketEncoding:  firstNonEmpty(query.Get("packetEncoding"), query.Get("packet-encoding")),
		Encryption:      query.Get("encryption"),
		ALPN:            splitCSV(query.Get("alpn")),
		ServerName:      firstNonEmpty(query.Get("sni"), query.Get("servername"), query.Get("serverName")),
		Fingerprint:     firstNonEmpty(query.Get("fp"), query.Get("fingerprint"), query.Get("client-fingerprint")),
		Path:            query.Get("path"),
		Host:            query.Get("host"),
		Mode:            query.Get("mode"),
		ServiceName:     firstNonEmpty(query.Get("serviceName"), query.Get("service-name")),
		HeaderType:      query.Get("type"),
		DownloadSetting: download,
	}
	if hasQuery(query, "insecure", "allowInsecure", "skip-cert-verify") {
		value := truthy(firstNonEmpty(query.Get("insecure"), query.Get("allowInsecure"), query.Get("skip-cert-verify")))
		info.SkipCertVerify = &value
	}
	info.RealityOpts = realityOpts(query)
	return info, nil
}

func parseTrojan(raw string) (uriInfo, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return uriInfo{}, err
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil || parsed.Hostname() == "" || parsed.User == nil {
		return uriInfo{}, fmt.Errorf("invalid trojan uri")
	}
	query := parsed.Query()
	info := uriInfo{
		Scheme:      "trojan",
		Server:      parsed.Hostname(),
		Port:        port,
		Password:    parsed.User.Username(),
		Network:     defaultString(query.Get("type"), "tcp"),
		ALPN:        splitCSV(query.Get("alpn")),
		ServerName:  firstNonEmpty(query.Get("sni"), query.Get("servername"), query.Get("serverName")),
		Fingerprint: firstNonEmpty(query.Get("fp"), query.Get("fingerprint"), query.Get("client-fingerprint")),
		Path:        query.Get("path"),
		Host:        query.Get("host"),
		ServiceName: firstNonEmpty(query.Get("serviceName"), query.Get("service-name")),
		HeaderType:  query.Get("type"),
		RealityOpts: nil,
	}
	if hasQuery(query, "insecure", "allowInsecure", "skip-cert-verify") {
		value := truthy(firstNonEmpty(query.Get("insecure"), query.Get("allowInsecure"), query.Get("skip-cert-verify")))
		info.SkipCertVerify = &value
	}
	return info, nil
}

func parseSS(raw string) (uriInfo, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return uriInfo{}, err
	}
	method, password, server, port, err := decodeSSAuthority(raw)
	if err != nil {
		return uriInfo{}, err
	}
	info := uriInfo{
		Scheme:   "ss",
		Server:   server,
		Port:     port,
		Cipher:   method,
		Password: password,
	}
	plugin := parsed.Query().Get("plugin")
	if plugin != "" {
		info.Plugin, info.PluginOpts = parseSSPlugin(plugin)
	}
	return info, nil
}

func parseVMess(raw string) (uriInfo, error) {
	payload, err := parseJSONFromVmess(raw)
	if err != nil {
		return uriInfo{}, err
	}
	port, err := strconv.Atoi(anyString(payload["port"]))
	if err != nil {
		return uriInfo{}, fmt.Errorf("invalid vmess uri")
	}
	info := uriInfo{
		Scheme:      "vmess",
		Server:      firstNonEmpty(anyString(payload["add"]), anyString(payload["server"]), anyString(payload["address"])),
		Port:        port,
		UUID:        firstNonEmpty(anyString(payload["id"]), anyString(payload["uuid"])),
		Network:     defaultString(strings.ToLower(firstNonEmpty(anyString(payload["net"]), anyString(payload["network"]))), "tcp"),
		AlterID:     intFromAny(payload["aid"]),
		Cipher:      firstNonEmpty(anyString(payload["scy"]), anyString(payload["cipher"]), "auto"),
		TLS:         strings.ToLower(firstNonEmpty(anyString(payload["tls"]), anyString(payload["security"]))) == "tls",
		ServerName:  firstNonEmpty(anyString(payload["sni"]), anyString(payload["servername"])),
		Fingerprint: firstNonEmpty(anyString(payload["fp"]), anyString(payload["fingerprint"])),
		ALPN:        splitCSV(anyString(payload["alpn"])),
		Host:        anyString(payload["host"]),
		Path:        anyString(payload["path"]),
		ServiceName: firstNonEmpty(anyString(payload["serviceName"]), anyString(payload["service-name"]), strings.TrimPrefix(anyString(payload["path"]), "/")),
		HeaderType:  anyString(payload["type"]),
	}
	if insecure := firstNonEmpty(anyString(payload["allowInsecure"]), anyString(payload["insecure"])); insecure != "" {
		value := truthy(insecure)
		info.SkipCertVerify = &value
	}
	if info.Server == "" || info.UUID == "" {
		return uriInfo{}, fmt.Errorf("invalid vmess uri")
	}
	return info, nil
}

func parseJSONFromVmess(raw string) (map[string]any, error) {
	payload := strings.TrimPrefix(strings.TrimSpace(raw), "vmess://")
	payload = strings.SplitN(payload, "#", 2)[0]
	payload = strings.SplitN(payload, "?", 2)[0]
	decoded, err := b64decodePadded(payload)
	if err != nil {
		return nil, fmt.Errorf("invalid vmess payload")
	}
	var data map[string]any
	if err := json.Unmarshal(decoded, &data); err != nil {
		return nil, fmt.Errorf("invalid vmess payload")
	}
	return data, nil
}

func b64decodePadded(text string) ([]byte, error) {
	raw := strings.TrimSpace(text)
	for len(raw)%4 != 0 {
		raw += "="
	}
	return base64.StdEncoding.DecodeString(raw)
}

func decodeSSAuthority(raw string) (string, string, string, int, error) {
	remainder := strings.TrimPrefix(strings.TrimSpace(raw), "ss://")
	remainder = strings.SplitN(remainder, "#", 2)[0]
	remainder = strings.SplitN(remainder, "?", 2)[0]
	decoded := remainder
	if !strings.Contains(remainder, "@") {
		body, err := b64decodePadded(remainder)
		if err != nil {
			return "", "", "", 0, err
		}
		decoded = string(body)
	} else {
		prefix, suffix, _ := strings.Cut(remainder, "@")
		body, err := b64decodePadded(prefix)
		if err == nil && !strings.Contains(string(body), "@") {
			decoded = string(body) + "@" + suffix
		}
	}
	creds, hostPort, ok := strings.Cut(decoded, "@")
	if !ok {
		return "", "", "", 0, fmt.Errorf("invalid ss uri")
	}
	method, password, ok := strings.Cut(creds, ":")
	if !ok {
		return "", "", "", 0, fmt.Errorf("invalid ss uri")
	}
	server, portText, ok := strings.Cut(hostPort, ":")
	if !ok {
		return "", "", "", 0, fmt.Errorf("invalid ss uri")
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		return "", "", "", 0, fmt.Errorf("invalid ss uri")
	}
	return method, password, server, port, nil
}

func parseSSPlugin(value string) (string, map[string]any) {
	parts := strings.Split(value, ";")
	plugin := strings.TrimSpace(parts[0])
	opts := map[string]any{}
	for _, item := range parts[1:] {
		key, val, ok := strings.Cut(item, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		if key == "tls" || key == "mux" {
			opts[key] = truthy(val)
		} else {
			opts[key] = val
		}
	}
	return plugin, opts
}

func xhttpDownloadSettings(mapping map[string]any) map[string]any {
	if len(mapping) == 0 {
		return nil
	}
	result := map[string]any{}
	if xs, ok := mapping["xhttpSettings"].(map[string]any); ok {
		if path := anyString(xs["path"]); path != "" {
			result["path"] = path
		}
		if host := anyString(xs["host"]); host != "" {
			result["host"] = host
		}
		if mode := anyString(xs["mode"]); mode != "" {
			result["mode"] = mode
		}
	}
	if server := firstNonEmpty(anyString(mapping["address"]), anyString(mapping["server"])); server != "" {
		result["server"] = server
	}
	if port := intFromAny(mapping["port"]); port != 0 {
		result["port"] = port
	}
	security := strings.ToLower(anyString(mapping["security"]))
	if security == "tls" || security == "reality" {
		result["tls"] = true
	}
	if security == "reality" {
		if rs, ok := mapping["realitySettings"].(map[string]any); ok {
			if serverName := firstNonEmpty(anyString(rs["serverName"]), anyString(rs["servername"]), anyString(rs["sni"])); serverName != "" {
				result["servername"] = serverName
			}
			if fp := firstNonEmpty(anyString(rs["fingerprint"]), anyString(rs["fp"])); fp != "" {
				result["client-fingerprint"] = fp
			}
			reality := map[string]any{}
			if pk := firstNonEmpty(anyString(rs["publicKey"]), anyString(rs["public-key"])); pk != "" {
				reality["public-key"] = pk
			}
			if sid := firstNonEmpty(anyString(rs["shortId"]), anyString(rs["short-id"])); sid != "" {
				reality["short-id"] = sid
			}
			if spx := firstNonEmpty(anyString(rs["spiderX"]), anyString(rs["spider-x"])); spx != "" {
				reality["spider-x"] = spx
			}
			if len(reality) > 0 {
				result["reality-opts"] = reality
			}
		}
	} else {
		if serverName := firstNonEmpty(anyString(mapping["serverName"]), anyString(mapping["servername"]), anyString(mapping["sni"])); serverName != "" {
			result["servername"] = serverName
		}
		if fp := firstNonEmpty(anyString(mapping["fingerprint"]), anyString(mapping["fp"])); fp != "" {
			result["client-fingerprint"] = fp
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func realityOpts(query url.Values) map[string]any {
	result := map[string]any{}
	if pk := firstNonEmpty(query.Get("pbk"), query.Get("publicKey"), query.Get("public-key")); pk != "" {
		result["public-key"] = pk
	}
	if sid := firstNonEmpty(query.Get("sid"), query.Get("shortId"), query.Get("short-id")); sid != "" {
		result["short-id"] = sid
	}
	if spx := firstNonEmpty(query.Get("spx"), query.Get("spiderX"), query.Get("spider-x")); spx != "" {
		result["spider-x"] = spx
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func guessName(uri string) string {
	raw := strings.TrimSpace(uri)
	if strings.HasPrefix(raw, "vmess://") {
		data, err := parseJSONFromVmess(raw)
		if err == nil {
			if name := anyString(data["ps"]); name != "" {
				return name
			}
			host := firstNonEmpty(anyString(data["add"]), anyString(data["server"]), anyString(data["address"]))
			network := defaultString(strings.ToLower(firstNonEmpty(anyString(data["net"]), anyString(data["network"]))), "tcp")
			if host != "" {
				return network + "-" + host
			}
		}
		return "vmess-node"
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "node"
	}
	if parsed.Fragment != "" {
		return parsed.Fragment
	}
	host := parsed.Hostname()
	if host == "" {
		host = "node"
	}
	if parsed.Scheme == "ss" {
		return "ss-" + host
	}
	network := parsed.Query().Get("type")
	if network == "" {
		network = "tcp"
	}
	return network + "-" + host
}

func uriScheme(raw string) string {
	if parts := strings.SplitN(strings.TrimSpace(raw), "://", 2); len(parts) == 2 {
		return strings.ToLower(parts[0])
	}
	return ""
}

func splitHost(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	return parsed.Hostname()
}

func splitPort(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	return parsed.Port()
}

func securityName(info uriInfo) string {
	switch info.Scheme {
	case "vless":
		if len(info.RealityOpts) > 0 {
			return "reality"
		}
		if info.ServerName != "" || len(info.ALPN) > 0 || info.SkipCertVerify != nil || len(info.DownloadSetting) > 0 {
			return "tls"
		}
		return "vless"
	case "trojan":
		if info.Plugin != "" {
			return info.Plugin
		}
		return "tls"
	case "vmess":
		if info.TLS {
			return "tls"
		}
		return "vmess"
	case "ss":
		return "ss"
	default:
		return info.Scheme
	}
}

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func splitCSV(value string) []string {
	parts := []string{}
	for _, item := range strings.Split(value, ",") {
		item = strings.TrimSpace(item)
		if item != "" {
			parts = append(parts, item)
		}
	}
	return parts
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func truthy(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func hasQuery(query url.Values, names ...string) bool {
	for _, name := range names {
		if _, ok := query[name]; ok {
			return true
		}
	}
	return false
}

func anyString(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case float64:
		return strconv.Itoa(int(v))
	case int:
		return strconv.Itoa(v)
	default:
		return ""
	}
}

func intFromAny(value any) int {
	switch v := value.(type) {
	case int:
		return v
	case float64:
		return int(v)
	case string:
		n, _ := strconv.Atoi(v)
		return n
	default:
		return 0
	}
}

func queryField(raw, key, fallback string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fallback
	}
	return defaultString(parsed.Query().Get(key), fallback)
}

func splitLines(text string) []string {
	lines := []string{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func AppendImportedNodes(existing []state.Node, rows []ScanRow, sourceKind, sourceID string, enabled bool) []state.Node {
	keys := map[string]struct{}{}
	names := []string{}
	for _, node := range existing {
		keys[URIBaseKey(node.URI)] = struct{}{}
		names = append(names, node.Name)
	}
	for _, row := range rows {
		if row.Supported != "1" {
			continue
		}
		key := URIBaseKey(row.URI)
		if _, ok := keys[key]; ok {
			continue
		}
		keys[key] = struct{}{}
		name := uniqueName(names, row.Name)
		names = append(names, name)
		existing = append(existing, state.Node{
			ID:         newProviderID(),
			Name:       name,
			Enabled:    enabled,
			URI:        strings.TrimSpace(row.URI),
			ImportedAt: state.NowISO(),
			Source:     state.Source{Kind: sourceKind, ID: sourceID},
		})
	}
	return existing
}

func uniqueName(existing []string, preferred string) string {
	if preferred == "" {
		preferred = "node"
	}
	if !slices.Contains(existing, preferred) {
		return preferred
	}
	for idx := 2; ; idx++ {
		candidate := fmt.Sprintf("%s-%d", preferred, idx)
		if !slices.Contains(existing, candidate) {
			return candidate
		}
	}
}

func newProviderID() string {
	return strconv.FormatInt(timeNow().UnixNano(), 36)
}

var timeNow = func() time.Time { return time.Now().UTC() }
