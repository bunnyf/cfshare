package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

// FormatLiveStatus 生成实时动态状态（用于持续刷新显示）
func (s *State) FormatLiveStatus() string {
	if s == nil {
		return "当前无活动分享\n\n用法: cfshare <path>... [--public] [--pass <password>]"
	}

	now := time.Now()
	uptime := now.Sub(s.StartTime)

	var b strings.Builder

	b.WriteString("分享状态 (实时)")
	b.WriteString("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	// 服务状态
	serverAlive := isProcessAlive(s.ServerPID)
	tunnelAlive := isProcessAlive(s.TunnelPID)

	if serverAlive && tunnelAlive {
		b.WriteString("状态:       🟢 在线\n")
	} else if serverAlive {
		b.WriteString("状态:       🟡 服务运行中 (隧道断开)\n")
	} else {
		b.WriteString("状态:       🔴 已停止\n")
	}

	b.WriteString(fmt.Sprintf("运行时间:   %s\n", formatDuration(uptime)))
	b.WriteString(fmt.Sprintf("URL:        %s\n", s.PublicURL))
	b.WriteString(fmt.Sprintf("模式:       %s\n", s.Mode))

	// 文件列表
	if s.IsMulti {
		b.WriteString(fmt.Sprintf("分享项:     %d 个项目\n", len(s.Items)))
		for i, item := range s.Items {
			sizeStr := ""
			if item.ShareType == TypeFile {
				sizeStr = fmt.Sprintf(" [%s]", formatBytes(item.Size))
			}
			b.WriteString(fmt.Sprintf("  [%d] %s (%s)%s\n", i+1, item.Name, item.ShareType, sizeStr))
		}
	} else if len(s.Items) > 0 {
		b.WriteString(fmt.Sprintf("路径:       %s\n", s.Items[0].Path))
		b.WriteString(fmt.Sprintf("类型:       %s\n", s.Items[0].ShareType))
		if s.Items[0].ShareType == TypeFile && s.Items[0].Size > 0 {
			b.WriteString(fmt.Sprintf("大小:       %s\n", formatBytes(s.Items[0].Size)))
		}
	}

	if s.Mode == ModeProtected {
		b.WriteString(fmt.Sprintf("用户名:     %s\n", s.Username))
		b.WriteString(fmt.Sprintf("密码:       %s\n", s.Password))
	}

	// 进程信息
	b.WriteString(fmt.Sprintf("\n进程信息\n"))
	b.WriteString("────────────────────────────────────────\n")
	serverStatus := "❌ 已停止"
	if serverAlive {
		serverStatus = fmt.Sprintf("✅ PID %d", s.ServerPID)
	}
	tunnelStatus := "❌ 已停止"
	if tunnelAlive {
		tunnelStatus = fmt.Sprintf("✅ PID %d", s.TunnelPID)
	}
	b.WriteString(fmt.Sprintf("服务器:     %s\n", serverStatus))
	b.WriteString(fmt.Sprintf("隧道:       %s\n", tunnelStatus))
	b.WriteString(fmt.Sprintf("端口:       %d\n", s.Port))

	// 访问统计（实时从文件读取）
	stats := LoadStatsData()

	b.WriteString(fmt.Sprintf("\n流量统计\n"))
	b.WriteString("────────────────────────────────────────\n")
	b.WriteString(fmt.Sprintf("总请求数:   %d\n", stats.RequestCount))
	b.WriteString(fmt.Sprintf("总传输:     %s\n", formatBytes(stats.TotalBytesSent)))

	// 计算近期速度
	if len(stats.RecentAccess) > 0 {
		speed := calculateSpeed(stats.RecentAccess)
		b.WriteString(fmt.Sprintf("当前速度:   %s/s\n", formatBytes(speed)))

		lastTime := stats.RecentAccess[len(stats.RecentAccess)-1].Time
		ago := now.Sub(lastTime)
		b.WriteString(fmt.Sprintf("最近访问:   %s前\n", formatDuration(ago)))
	} else {
		b.WriteString("当前速度:   --\n")
		b.WriteString("最近访问:   无\n")
	}

	// 最近的访问记录
	if len(stats.RecentAccess) > 0 {
		b.WriteString(fmt.Sprintf("\n最近请求\n"))
		b.WriteString("────────────────────────────────────────\n")
		start := 0
		if len(stats.RecentAccess) > 5 {
			start = len(stats.RecentAccess) - 5
		}
		for _, rec := range stats.RecentAccess[start:] {
			statusIcon := "✅"
			if rec.StatusCode >= 400 {
				statusIcon = "❌"
			}
			b.WriteString(fmt.Sprintf("  %s %s %s [%d] %s\n",
				statusIcon,
				rec.Time.Format("15:04:05"),
				rec.Path,
				rec.StatusCode,
				formatBytes(rec.BytesSent),
			))
		}
	}

	b.WriteString("\n按 Ctrl+C 退出实时监控")

	return b.String()
}

// calculateSpeed 根据最近的访问记录计算传输速度（字节/秒）
func calculateSpeed(records []AccessRecord) int64 {
	if len(records) == 0 {
		return 0
	}

	now := time.Now()
	window := 30 * time.Second // 30秒窗口

	var totalBytes int64
	cutoff := now.Add(-window)

	for _, r := range records {
		if r.Time.After(cutoff) {
			totalBytes += r.BytesSent
		}
	}

	elapsed := now.Sub(cutoff).Seconds()
	if elapsed <= 0 {
		return 0
	}

	return int64(float64(totalBytes) / elapsed)
}

// formatDuration 格式化持续时间为人类可读格式
func formatDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}

	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60

	if days > 0 {
		return fmt.Sprintf("%d天 %d时 %d分", days, hours, minutes)
	}
	if hours > 0 {
		return fmt.Sprintf("%d时 %d分 %d秒", hours, minutes, seconds)
	}
	if minutes > 0 {
		return fmt.Sprintf("%d分 %d秒", minutes, seconds)
	}
	return fmt.Sprintf("%d秒", seconds)
}

// formatBytes 格式化字节数为人类可读格式
func formatBytes(b int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
		TB = GB * 1024
	)

	switch {
	case b >= TB:
		return fmt.Sprintf("%.2f TB", float64(b)/float64(TB))
	case b >= GB:
		return fmt.Sprintf("%.2f GB", float64(b)/float64(GB))
	case b >= MB:
		return fmt.Sprintf("%.2f MB", float64(b)/float64(MB))
	case b >= KB:
		return fmt.Sprintf("%.2f KB", float64(b)/float64(KB))
	default:
		return fmt.Sprintf("%d B", b)
	}
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


// StatsData 访问统计数据
type StatsData struct {
	RequestCount   int            `json:"request_count"`
	TotalBytesSent int64          `json:"total_bytes_sent"`
	LastAccess     time.Time      `json:"last_access,omitempty"`
	RecentAccess   []AccessRecord `json:"recent_access,omitempty"`
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
	var stats StatsData
	data, _ := os.ReadFile(statsPath)
	json.Unmarshal(data, &stats)

	// 更新统计
	stats.RequestCount++
	stats.TotalBytesSent += record.BytesSent
	stats.LastAccess = record.Time
	stats.RecentAccess = append(stats.RecentAccess, record)
	if len(stats.RecentAccess) > 50 {
		stats.RecentAccess = stats.RecentAccess[len(stats.RecentAccess)-50:]
	}

	// 写回
	newData, _ := json.MarshalIndent(stats, "", "  ")
	f.Truncate(0)
	f.Seek(0, 0)
	f.Write(newData)

	return nil
}

// LoadStats 加载访问统计（向后兼容）
func LoadStats() (requestCount int, lastAccess time.Time, recentAccess []AccessRecord) {
	stats := LoadStatsData()
	return stats.RequestCount, stats.LastAccess, stats.RecentAccess
}

// LoadStatsData 加载完整的访问统计数据
func LoadStatsData() StatsData {
	statsPath := config.GetConfigDir() + "/stats.json"
	data, err := os.ReadFile(statsPath)
	if err != nil {
		return StatsData{}
	}
	var stats StatsData
	json.Unmarshal(data, &stats)
	return stats
}

