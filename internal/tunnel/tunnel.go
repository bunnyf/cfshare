package tunnel

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"cfshare/internal/config"
)

type Manager struct {
	tunnelName string
	configPath string
}

func NewManager(tunnelName string) *Manager {
	return &Manager{
		tunnelName: tunnelName,
	}
}

func (m *Manager) Start() (int, error) {
	cloudflaredPath, err := exec.LookPath("cloudflared")
	if err != nil {
		return 0, fmt.Errorf("cloudflared not found in PATH: %w\n请先安装 cloudflared: https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/downloads/", err)
	}

	if pid := m.GetRunningPID(); pid > 0 {
		return pid, nil
	}

	// 使用 http2 协议，避免 QUIC 在某些网络环境下被阻止
	cmd := exec.Command(cloudflaredPath, "tunnel", "--protocol", "http2", "run", m.tunnelName)

	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	logPath := config.GetConfigDir() + "/tunnel.log"
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return 0, fmt.Errorf("create tunnel log file: %w", err)
	}

	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if err := cmd.Start(); err != nil {
		logFile.Close()
		return 0, fmt.Errorf("start cloudflared: %w", err)
	}

	pid := cmd.Process.Pid
	if err := m.savePID(pid); err != nil {
		cmd.Process.Kill()
		logFile.Close()
		return pid, fmt.Errorf("save tunnel pid: %w", err)
	}

	time.Sleep(500 * time.Millisecond)

	if !m.isProcessRunning(pid) {
		return 0, fmt.Errorf("tunnel process died immediately, check %s for details", logPath)
	}

	return pid, nil
}

func (m *Manager) Stop() error {
	pid := m.GetRunningPID()
	if pid <= 0 {
		return nil
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		m.removePIDFile()
		return nil
	}

	if err := process.Signal(syscall.SIGTERM); err != nil {
		m.removePIDFile()
		return nil
	}

	done := make(chan error, 1)
	go func() {
		_, err := process.Wait()
		done <- err
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		process.Signal(syscall.SIGKILL)
	}

	m.removePIDFile()
	return nil
}

func (m *Manager) ForceStop() error {
	pid := m.GetRunningPID()
	if pid <= 0 {
		m.removePIDFile()
		return nil
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		m.removePIDFile()
		return nil
	}

	process.Signal(syscall.SIGKILL)
	m.removePIDFile()
	return nil
}

func (m *Manager) GetRunningPID() int {
	data, err := os.ReadFile(config.GetTunnelPidFilePath())
	if err != nil {
		return 0
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0
	}

	if !m.isProcessRunning(pid) {
		m.removePIDFile()
		return 0
	}

	return pid
}

func (m *Manager) IsRunning() bool {
	return m.GetRunningPID() > 0
}

func (m *Manager) savePID(pid int) error {
	return os.WriteFile(config.GetTunnelPidFilePath(), []byte(strconv.Itoa(pid)), 0600)
}

func (m *Manager) removePIDFile() {
	os.Remove(config.GetTunnelPidFilePath())
}

func (m *Manager) isProcessRunning(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

func (m *Manager) GetPublicURL() (string, error) {
	home, _ := os.UserHomeDir()
	configPaths := []string{
		home + "/.cloudflared/config.yml",
		home + "/.cloudflared/config.yaml",
		"/etc/cloudflared/config.yml",
		"/etc/cloudflared/config.yaml",
	}

	for _, configPath := range configPaths {
		if url := parseHostnameFromConfig(configPath); url != "" {
			return "https://" + url, nil
		}
	}

	return m.getURLFromTunnelInfo()
}

func parseHostnameFromConfig(configPath string) string {
	file, err := os.Open(configPath)
	if err != nil {
		return ""
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "hostname:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1])
			}
		}
		if strings.Contains(line, "hostname:") {
			parts := strings.SplitN(line, "hostname:", 2)
			if len(parts) == 2 {
				hostname := strings.TrimSpace(parts[1])
				hostname = strings.Trim(hostname, "\"'")
				if hostname != "" && hostname != "*" {
					return hostname
				}
			}
		}
	}

	return ""
}

func (m *Manager) getURLFromTunnelInfo() (string, error) {
	cmd := exec.Command("cloudflared", "tunnel", "info", m.tunnelName)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("get tunnel info: %w", err)
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "Connector") {
			continue
		}
		if strings.Contains(line, ".") && !strings.Contains(line, "Connector") {
			parts := strings.Fields(line)
			for _, part := range parts {
				if strings.Contains(part, ".") && !strings.HasPrefix(part, "10.") && !strings.HasPrefix(part, "192.") {
					return "https://" + strings.Trim(part, "\"'"), nil
				}
			}
		}
	}

	return "", fmt.Errorf("could not determine public URL, please set it in config")
}

func CheckSetup(tunnelName string) error {
	if _, err := exec.LookPath("cloudflared"); err != nil {
		return fmt.Errorf("cloudflared 未安装\n\n请先安装:\n  macOS: brew install cloudflared\n  Linux: 参考 https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/downloads/")
	}

	cmd := exec.Command("cloudflared", "tunnel", "list")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("无法获取 tunnel 列表: %w\n\n请先登录: cloudflared tunnel login", err)
	}

	if !strings.Contains(string(output), tunnelName) {
		return fmt.Errorf("tunnel '%s' 不存在\n\n请先创建:\n  cloudflared tunnel create %s\n  然后配置 DNS route 和 config.yml", tunnelName, tunnelName)
	}

	return nil
}
