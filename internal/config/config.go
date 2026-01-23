package config

import (
	"os"
	"path/filepath"
)

const (
	DefaultPort       = 8787
	DefaultUsername   = "dl"
	PasswordLength    = 16
	StateFileName     = "state.json"
	AccessLogFileName = "access.log"
	TunnelName        = "cfshare"
)

func GetConfigDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".cfshare"
	}
	return filepath.Join(home, ".cfshare")
}

func GetStatePath() string {
	return filepath.Join(GetConfigDir(), StateFileName)
}

func GetAccessLogPath() string {
	return filepath.Join(GetConfigDir(), AccessLogFileName)
}

func GetPidFilePath() string {
	return filepath.Join(GetConfigDir(), "server.pid")
}

func GetTunnelPidFilePath() string {
	return filepath.Join(GetConfigDir(), "tunnel.pid")
}

func EnsureConfigDir() error {
	return os.MkdirAll(GetConfigDir(), 0700)
}

func GetStatsPath() string {
	return filepath.Join(GetConfigDir(), "stats.json")
}
