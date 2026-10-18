package core

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Settings 应用设置
type Settings struct {
	MaxConcurrentTasks int    `json:"maxConcurrentTasks"` // 同时下载的任务数（默认 3）
	ConcurrencyPerTask int    `json:"concurrencyPerTask"` // 单个任务内分片并发数（默认 10）
	SavePath           string `json:"savePath"`           // 保存路径
	ApiBase            string `json:"apiBaseUrl"`         // 接口地址
	ApiKey             string `json:"apiKey"`             // 授权密钥
}

// DefaultSettings 返回默认设置
func DefaultSettings() Settings {
	return Settings{
		MaxConcurrentTasks: 3,
		ConcurrencyPerTask: 10,
		SavePath:           "",
	}
}

// settingsPath 设置文件的本地路径（应用数据目录）。
func settingsFilePath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "desk-m3u8-settings.json"
	}
	dir = filepath.Join(dir, "desk-m3u8")
	_ = os.MkdirAll(dir, 0o755)
	return filepath.Join(dir, "settings.json")
}

// LoadSettings 从磁盘读取设置，读取失败时返回默认设置。
func LoadSettings() Settings {
	s := DefaultSettings()
	data, err := os.ReadFile(settingsFilePath())
	if err != nil {
		return s
	}
	_ = json.Unmarshal(data, &s)
	// 兜底默认值
	if s.MaxConcurrentTasks <= 0 {
		s.MaxConcurrentTasks = 3
	}
	if s.ConcurrencyPerTask <= 0 {
		s.ConcurrencyPerTask = 10
	}
	return s
}

// SaveSettings 持久化设置到磁盘。
func SaveSettings(s Settings) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(settingsFilePath(), data, 0o644)
}
