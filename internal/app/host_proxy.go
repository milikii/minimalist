package app

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"

	"minimalist/internal/config"
)

// HostProxyStatus prints the current host OUTPUT proxy mode from config truth.
func (a *App) HostProxyStatus() error {
	cfg, _, err := a.ensureAll()
	if err != nil {
		return operatorActionError("读取宿主机接管状态失败", err, "先执行 minimalist status", "README.md")
	}
	if cfg.Network.ProxyHostOutput {
		fmt.Fprintln(a.Stdout, "宿主机流量接管: on")
		return nil
	}
	fmt.Fprintln(a.Stdout, "宿主机流量接管: off")
	return nil
}

// HostProxyEnable turns on host OUTPUT proxy after confirmation.
func (a *App) HostProxyEnable() error {
	return a.setHostProxy(true, bufio.NewReader(a.Stdin))
}

// HostProxyDisable turns off host OUTPUT proxy after confirmation.
func (a *App) HostProxyDisable() error {
	return a.setHostProxy(false, bufio.NewReader(a.Stdin))
}

func (a *App) setHostProxy(enabled bool, reader *bufio.Reader) error {
	if err := a.requireRoot(); err != nil {
		return operatorActionError("宿主机流量接管变更失败", err, "请用 root 执行 minimalist host-proxy enable|disable", "README.md")
	}
	if err := a.ensureCutoverReady(); err != nil {
		return operatorActionError("宿主机流量接管变更失败", err, "先执行 minimalist cutover-preflight", "docs/README_FLOWS.md")
	}
	cfg, st, err := a.ensureAll()
	if err != nil {
		return operatorActionError("读取宿主机接管配置失败", err, "先执行 minimalist status", "README.md")
	}
	if enabled && !a.hasEnabledManualNodes(st) {
		return operatorActionError("宿主机流量接管变更失败", errors.New("没有启用的手动节点"), "先执行 minimalist nodes enable <index>", "README.md")
	}
	if cfg.Network.ProxyHostOutput == enabled {
		if enabled {
			fmt.Fprintln(a.Stdout, "宿主机流量接管已是 on")
		} else {
			fmt.Fprintln(a.Stdout, "宿主机流量接管已是 off")
		}
		return nil
	}
	if !confirmHostProxyChange(reader, a.Stdout, enabled) {
		fmt.Fprintln(a.Stdout, "已取消宿主机流量接管变更")
		return nil
	}

	previous := cfg
	cfg.Network.ProxyHostOutput = enabled
	if err := a.persistHostProxyConfig(cfg); err != nil {
		if rollbackErr := a.rollbackHostProxy(previous); rollbackErr != nil {
			return operatorActionError("宿主机流量接管变更失败，且回滚未完成", fmt.Errorf("%w; rollback failed: %v", err, rollbackErr), "立即执行 minimalist host-proxy status && minimalist runtime-audit", "docs/README_FLOWS.md")
		}
		return operatorActionError("宿主机流量接管变更失败，但已恢复旧配置", err, "重新执行 minimalist host-proxy status 确认状态", "docs/README_FLOWS.md")
	}

	if enabled {
		fmt.Fprintln(a.Stdout, "宿主机流量接管已开启")
	} else {
		fmt.Fprintln(a.Stdout, "宿主机流量接管已关闭")
	}
	return nil
}

func (a *App) persistHostProxyConfig(cfg config.Config) error {
	if err := config.Save(a.Paths.ConfigPath(), cfg); err != nil {
		return err
	}
	if err := a.RenderConfig(); err != nil {
		return fmt.Errorf("render host-proxy config: %w", err)
	}
	if err := a.ApplyRules(); err != nil {
		return fmt.Errorf("apply host-proxy rules: %w", err)
	}
	return nil
}

func (a *App) rollbackHostProxy(cfg config.Config) error {
	if err := config.Save(a.Paths.ConfigPath(), cfg); err != nil {
		return err
	}
	if err := a.RenderConfig(); err != nil {
		return err
	}
	if err := a.ApplyRules(); err != nil {
		return err
	}
	return nil
}

func confirmHostProxyChange(reader *bufio.Reader, out io.Writer, enabled bool) bool {
	label := "确认关闭宿主机流量接管"
	if enabled {
		label = "确认开启宿主机流量接管"
	}
	fmt.Fprintf(out, "%s [y/N]: ", label)
	line, err := readChoice(reader)
	if err != nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true
	default:
		return false
	}
}
