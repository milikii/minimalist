package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	yaml "gopkg.in/yaml.v2"
)

type Config struct {
	Version    int        `yaml:"version"`
	Profile    Profile    `yaml:"profile"`
	Network    Network    `yaml:"network"`
	Ports      Ports      `yaml:"ports"`
	Controller Controller `yaml:"controller"`
	Access     Access     `yaml:"access"`
	Install    Install    `yaml:"install"`
}

type Profile struct {
	Template   string `yaml:"template"`
	Mode       string `yaml:"mode"`
	RulePreset string `yaml:"rule_preset"`
}

type Network struct {
	EnableIPv6             bool     `yaml:"enable_ipv6"`
	LANInterfaces          []string `yaml:"lan_interfaces"`
	LANCIDRs               []string `yaml:"lan_cidrs"`
	ProxyIngressInterfaces []string `yaml:"proxy_ingress_interfaces"`
	DNSHijackEnabled       bool     `yaml:"dns_hijack_enabled"`
	DNSHijackInterfaces    []string `yaml:"dns_hijack_interfaces"`
	ProxyHostOutput        bool     `yaml:"proxy_host_output"`
	Bypass                 Bypass   `yaml:"bypass"`
}

type Bypass struct {
	ContainerNames []string `yaml:"container_names"`
	SrcCIDRs       []string `yaml:"src_cidrs"`
	DstCIDRs       []string `yaml:"dst_cidrs"`
	UIDs           []string `yaml:"uids"`
}

type Ports struct {
	Mixed      int `yaml:"mixed"`
	TProxy     int `yaml:"tproxy"`
	DNS        int `yaml:"dns"`
	Controller int `yaml:"controller"`
}

type Controller struct {
	BindAddress             string   `yaml:"bind_address"`
	Secret                  string   `yaml:"secret"`
	CORSAllowOrigins        []string `yaml:"cors_allow_origins"`
	CORSAllowPrivateNetwork bool     `yaml:"cors_allow_private_network"`
}

type Access struct {
	LANDisallowedCIDRs []string `yaml:"lan_disallowed_cidrs"`
	Authentication     []string `yaml:"authentication"`
	SkipAuthPrefixes   []string `yaml:"skip_auth_prefixes"`
}

type Install struct {
	CoreBin string `yaml:"core_bin"`
}

func Default() Config {
	return Config{
		Version: 1,
		Profile: Profile{
			Template:   "nas-single-lan-v4",
			Mode:       "rule",
			RulePreset: "default",
		},
		Network: Network{
			EnableIPv6:             false,
			LANInterfaces:          []string{"bridge1"},
			LANCIDRs:               []string{"192.168.2.0/24"},
			ProxyIngressInterfaces: []string{"bridge1"},
			DNSHijackEnabled:       true,
			DNSHijackInterfaces:    []string{"bridge1"},
			ProxyHostOutput:        false,
			Bypass:                 Bypass{},
		},
		Ports: Ports{
			Mixed:      7890,
			TProxy:     7893,
			DNS:        1053,
			Controller: 19090,
		},
		Controller: Controller{
			BindAddress:             "127.0.0.1",
			Secret:                  randomSecret(),
			CORSAllowOrigins:        nil,
			CORSAllowPrivateNetwork: false,
		},
		Access: Access{},
		Install: Install{
			CoreBin: "/usr/local/bin/mihomo-core",
		},
	}
}

func Ensure(path string) (Config, error) {
	cfg, err := Load(path)
	if err == nil {
		if cfg.Controller.Secret == "" {
			cfg.Controller.Secret = randomSecret()
			if err := Save(path, cfg); err != nil {
				return Config{}, err
			}
		}
		return cfg, nil
	}
	if !os.IsNotExist(err) {
		return Config{}, err
	}
	cfg = Default()
	return cfg, Save(path, cfg)
}

func Load(path string) (Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	cfg := Default()
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	if cfg.Controller.Secret == "" {
		cfg.Controller.Secret = randomSecret()
	}
	return cfg, nil
}

func Save(path string, cfg Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := yaml.Marshal(&cfg)
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	return os.WriteFile(path, raw, 0o640)
}

func randomSecret() string {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return "minimalist-secret"
	}
	return hex.EncodeToString(buf)
}
