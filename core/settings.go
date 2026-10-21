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
	ApiPort            int    `json:"apiPort"`            // 本地接口提交下载监听端口（默认 52323）
	ApiBase            string `json:"apiBaseUrl"`         // 接口地址
	ApiKey             string `json:"apiKey"`             // 授权密钥
}

// defaultSaveDir 返回默认下载目录：软件（可执行文件）所在目录。
// 首次启动（未配置过保存路径）时，下载目录自动设置为当前软件启动/安装的目录。
func defaultSaveDir() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return filepath.Dir(exe)
}

// DefaultSettings 返回默认设置
func DefaultSettings() Settings {
	return Settings{
		MaxConcurrentTasks: 3,
		ConcurrencyPerTask: 10,
		SavePath:           defaultSaveDir(),
		ApiPort:            52323,
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
	if s.ApiPort <= 0 || s.ApiPort > 65535 {
		s.ApiPort = 52323
	}
	if s.SavePath == "" {
		// 首次启动 / 未设置保存路径：默认使用软件所在目录
		s.SavePath = defaultSaveDir()
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
