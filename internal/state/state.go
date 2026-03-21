package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
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

// TransferRecord 记录一次活动文件传输的进度（服务进程写，CLI 进程读）
type TransferRecord struct {
	ID         string    `json:"id"`
	FileName   string    `json:"file_name"`
	TotalSize  int64     `json:"total_size"`
	BytesSent  int64     `json:"bytes_sent"`  // 本次连接已传字节
	RangeStart int64     `json:"range_start"` // 断点续传起始偏移
	StartTime  time.Time `json:"start_time"`
	RemoteAddr string    `json:"remote_addr"`
	Done       bool      `json:"done,omitempty"`
}

// BytesReceived 返回客户端实际已收到的总字节数（含续传偏移）
func (r *TransferRecord) BytesReceived() int64 {
	return r.RangeStart + r.BytesSent
}

// TransferSnapshot 是 transfers.json 的文件格式
type TransferSnapshot struct {
	Transfers []TransferRecord `json:"transfers"`
	UpdatedAt time.Time        `json:"updated_at"`
}

// LoadTransferSnapshot 读取传输进度快照，超过 3s 未更新视为失效
func LoadTransferSnapshot() (*TransferSnapshot, error) {
	data, err := os.ReadFile(config.GetTransfersPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var s TransferSnapshot
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	if time.Since(s.UpdatedAt) > 3*time.Second {
		return nil, nil
	}
	return &s, nil
}

// ShareItem 表示单个分享项
type ShareItem struct {
	Path      string    `json:"path"`       // 绝对路径
	Name      string    `json:"name"`       // 显示名称 (基础文件名)
	ShareType ShareType `json:"share_type"` // file 或 dir
	Size      int64     `json:"size"`       // 文件大小 (目录为 0)
}

type State struct {
	mu sync.RWMutex

	ShareID string    `json:"share_id"`
	Mode    ShareMode `json:"mode"`
	Port    int       `json:"port"`

	// 多路径支持
	Items   []ShareItem `json:"items,omitempty"`   // 分享项列表
	IsMulti bool        `json:"is_multi"`          // 是否多文件模式

	// 向后兼容 (单文件时填充)
	Path      string    `json:"path,omitempty"`
	ShareType ShareType `json:"share_type,omitempty"`

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

	// 向后兼容: 如果是旧格式 (Items 为空但 Path 有值)
	if len(s.Items) == 0 && s.Path != "" {
		s.Items = []ShareItem{{
			Path:      s.Path,
			Name:      filepath.Base(s.Path),
			ShareType: s.ShareType,
		}}
		s.IsMulti = false
	}

	return &s, nil
}

func (s *State) Save() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if err := config.EnsureConfigDir(); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	// 兼容性: 单文件时同步旧字段
	if len(s.Items) == 1 {
		s.Path = s.Items[0].Path
		s.ShareType = s.Items[0].ShareType
		s.IsMulti = false
	} else if len(s.Items) > 1 {
		s.IsMulti = true
		s.Path = ""
		s.ShareType = ""
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

	return isProcessAlive(s.ServerPID)
}

func (s *State) FormatStatus() string {
	if s == nil {
		return "当前无活动分享\n\n用法: cfshare <path>... [--public] [--pass <password>]"
	}

	status := fmt.Sprintf(`分享状态
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
URL:        %s
Mode:       %s
`, s.PublicURL, s.Mode)

	// 多文件显示
	if s.IsMulti {
		status += fmt.Sprintf("Items:      %d 个项目\n", len(s.Items))
		for i, item := range s.Items {
			status += fmt.Sprintf("  [%d] %s (%s) - %s\n", i+1, item.Name, item.ShareType, item.Path)
		}
	} else if len(s.Items) > 0 {
		status += fmt.Sprintf("Path:       %s\nType:       %s\n", s.Items[0].Path, s.Items[0].ShareType)
	} else {
		// 兼容旧格式
		status += fmt.Sprintf("Path:       %s\nType:       %s\n", s.Path, s.ShareType)
	}

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

	requestCount, lastAccess, _ := LoadStats()
	if requestCount > 0 {
		status += fmt.Sprintf(`
访问统计
────────────────────────────────────────
Requests:   %d
Last Access: %s
`, requestCount, lastAccess.Format("2006-01-02 15:04:05"))
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
Mode:     %s
`, s.PublicURL, s.Mode)

	// 多文件显示
	if s.IsMulti {
		output += fmt.Sprintf("Items:    %d 个项目\n", len(s.Items))
		for i, item := range s.Items {
			output += fmt.Sprintf("  [%d] %s (%s)\n", i+1, item.Name, item.ShareType)
		}
	} else if len(s.Items) > 0 {
		output += fmt.Sprintf("Path:     %s\nType:     %s\n", s.Items[0].Path, s.Items[0].ShareType)
	} else {
		output += fmt.Sprintf("Path:     %s\nType:     %s\n", s.Path, s.ShareType)
	}

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


// UpdateAccessStats 只更新访问统计（使用文件锁避免竞态）
func UpdateAccessStats(record AccessRecord) error {
	statsPath := config.GetConfigDir() + "/stats.json"
	
	// 打开或创建 stats 文件并加锁
	f, err := os.OpenFile(statsPath, os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	
	// 加文件锁
	if err := lockFile(f); err != nil {
		return err
	}
	defer unlockFile(f)
	
	// 读取现有统计
	var stats struct {
		RequestCount int            `json:"request_count"`
		LastAccess   time.Time      `json:"last_access,omitempty"`
		RecentAccess []AccessRecord `json:"recent_access,omitempty"`
	}
	
	data, _ := os.ReadFile(statsPath)
	json.Unmarshal(data, &stats)
	
	// 更新统计
	stats.RequestCount++
	stats.LastAccess = record.Time
	stats.RecentAccess = append(stats.RecentAccess, record)
	if len(stats.RecentAccess) > 10 {
		stats.RecentAccess = stats.RecentAccess[len(stats.RecentAccess)-10:]
	}
	
	// 写回
	newData, _ := json.MarshalIndent(stats, "", "  ")
	f.Truncate(0)
	f.Seek(0, 0)
	f.Write(newData)
	
	return nil
}

// LoadStats 加载访问统计
func LoadStats() (requestCount int, lastAccess time.Time, recentAccess []AccessRecord) {
	statsPath := config.GetConfigDir() + "/stats.json"
	data, err := os.ReadFile(statsPath)
	if err != nil {
		return
	}
	var stats struct {
		RequestCount int            `json:"request_count"`
		LastAccess   time.Time      `json:"last_access,omitempty"`
		RecentAccess []AccessRecord `json:"recent_access,omitempty"`
	}
	json.Unmarshal(data, &stats)
	return stats.RequestCount, stats.LastAccess, stats.RecentAccess
}

