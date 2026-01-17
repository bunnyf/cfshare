package state

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"syscall"
	"time"

	"cfshare/internal/config"
)

type ShareMode string

const (
	ModeProtected ShareMode = "protected"
	ModePublic    ShareMode = "public"
)

type ShareType string

const (
	TypeFile ShareType = "file"
	TypeDir  ShareType = "dir"
)

type AccessRecord struct {
	Time       time.Time `json:"time"`
	Path       string    `json:"path"`
	StatusCode int       `json:"status_code"`
	BytesSent  int64     `json:"bytes_sent"`
	RemoteAddr string    `json:"remote_addr"`
}

type State struct {
	mu sync.RWMutex

	ShareID   string    `json:"share_id"`
	Mode      ShareMode `json:"mode"`
	Path      string    `json:"path"`
	ShareType ShareType `json:"share_type"`
	Port      int       `json:"port"`

	ServerPID int `json:"server_pid"`
	TunnelPID int `json:"tunnel_pid"`

	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`

	StartTime  time.Time `json:"start_time"`
	LastAccess time.Time `json:"last_access,omitempty"`

	RequestCount int            `json:"request_count"`
	RecentAccess []AccessRecord `json:"recent_access,omitempty"`

	PublicURL string `json:"public_url"`
}

func Load() (*State, error) {
	path := config.GetStatePath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read state file: %w", err)
	}

	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse state file: %w", err)
	}

	return &s, nil
}

func (s *State) Save() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if err := config.EnsureConfigDir(); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	path := config.GetStatePath()
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("write state file: %w", err)
	}

	return nil
}

func Clear() error {
	path := config.GetStatePath()
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove state file: %w", err)
	}
	return nil
}

func (s *State) RecordAccess(record AccessRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.RequestCount++
	s.LastAccess = record.Time

	s.RecentAccess = append(s.RecentAccess, record)
	if len(s.RecentAccess) > 10 {
		s.RecentAccess = s.RecentAccess[len(s.RecentAccess)-10:]
	}
}

func (s *State) IsRunning() bool {
	if s == nil || s.ServerPID == 0 {
		return false
	}

	process, err := os.FindProcess(s.ServerPID)
	if err != nil {
		return false
	}

	err = process.Signal(syscall.Signal(0))
	return err == nil
}

func (s *State) FormatStatus() string {
	if s == nil {
		return "当前无活动分享\n\n用法: cfshare <path> [--public] [--pass <password>]"
	}

	status := fmt.Sprintf(`分享状态
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
URL:        %s
Path:       %s
Type:       %s
Mode:       %s
`, s.PublicURL, s.Path, s.ShareType, s.Mode)

	if s.Mode == ModeProtected {
		status += fmt.Sprintf(`Username:   %s
Password:   %s
`, s.Username, s.Password)
	}

	status += fmt.Sprintf(`
Service:    %s
Server PID: %d
Tunnel PID: %d
Port:       %d

Started:    %s
`, s.runningStatus(), s.ServerPID, s.TunnelPID, s.Port, s.StartTime.Format("2006-01-02 15:04:05"))

	if s.RequestCount > 0 {
		status += fmt.Sprintf(`
访问统计
────────────────────────────────────────
Requests:   %d
Last Access: %s
`, s.RequestCount, s.LastAccess.Format("2006-01-02 15:04:05"))
	}

	return status
}

func (s *State) runningStatus() string {
	if s.IsRunning() {
		return "🟢 服务运行中"
	}
	return "🔴 服务已停止"
}

func (s *State) FormatShareOutput() string {
	output := fmt.Sprintf(`
✅ 分享已启动

URL:      %s
Path:     %s
Type:     %s
Mode:     %s
`, s.PublicURL, s.Path, s.ShareType, s.Mode)

	if s.Mode == ModeProtected {
		output += fmt.Sprintf(`
Username: %s
Password: %s
`, s.Username, s.Password)
	} else {
		output += "\n⚠️  公开分享，任何人都可以访问\n"
	}

	return output
}

// UpdateAccessStats 只更新访问统计（不覆盖其他字段）
func UpdateAccessStats(record AccessRecord) error {
	// 加载现有状态
	existing, err := Load()
	if err != nil {
		return err
	}
	if existing == nil {
		return nil // 没有状态文件，跳过
	}

	// 只更新统计字段
	existing.mu.Lock()
	existing.RequestCount++
	existing.LastAccess = record.Time
	existing.RecentAccess = append(existing.RecentAccess, record)
	if len(existing.RecentAccess) > 10 {
		existing.RecentAccess = existing.RecentAccess[len(existing.RecentAccess)-10:]
	}
	existing.mu.Unlock()

	return existing.Save()
}
