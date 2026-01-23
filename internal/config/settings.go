package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// GuildSettings サーバーごとの設定
type GuildSettings struct {
	NotificationChannel  *string  `json:"notification_channel,omitempty"`   // 通知チャンネルID
	AutoNotifyEnabled    bool     `json:"auto_notify_enabled"`              // 自動通知ON/OFF
	NotificationDelay    float64  `json:"notification_delay"`               // 通知遅延（秒）
	NotificationThreshold float64 `json:"notification_threshold"`           // 通知閾値（%）
	MentionRole          *string  `json:"mention_role,omitempty"`           // メンションロールID
	MentionThreshold     float64  `json:"mention_threshold"`                // メンション閾値（%）
	NotificationMetric   string   `json:"notification_metric"`              // 通知指標: "overall" or "weighted"
}

// DefaultGuildSettings デフォルト設定
var DefaultGuildSettings = GuildSettings{
	AutoNotifyEnabled:     true,
	NotificationDelay:     0.5,
	NotificationThreshold: 10.0,
	MentionThreshold:      50.0,
	NotificationMetric:    "overall",
}

// SettingsManager 設定管理
type SettingsManager struct {
	mu       sync.RWMutex
	Guilds   map[string]GuildSettings `json:"guilds"`
	filePath string
}

// NewSettingsManager 設定マネージャーを作成
func NewSettingsManager(configPath string) *SettingsManager {
	sm := &SettingsManager{
		Guilds:   make(map[string]GuildSettings),
		filePath: configPath,
	}
	sm.Load()
	return sm
}

// Load 設定をファイルから読み込む
func (sm *SettingsManager) Load() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if _, err := os.Stat(sm.filePath); os.IsNotExist(err) {
		// ファイルが存在しない場合はデフォルト設定で保存
		return sm.saveUnsafe()
	}

	data, err := os.ReadFile(sm.filePath)
	if err != nil {
		return err
	}

	type FileFormat struct {
		Guilds map[string]GuildSettings `json:"guilds"`
	}

	var format FileFormat
	if err := json.Unmarshal(data, &format); err != nil {
		return err
	}

	sm.Guilds = format.Guilds
	if sm.Guilds == nil {
		sm.Guilds = make(map[string]GuildSettings)
	}

	return nil
}

// Save 設定をファイルに保存
func (sm *SettingsManager) Save() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	return sm.saveUnsafe()
}

func (sm *SettingsManager) saveUnsafe() error {
	// ディレクトリを作成
	dir := filepath.Dir(sm.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	type FileFormat struct {
		Guilds map[string]GuildSettings `json:"guilds"`
	}

	format := FileFormat{
		Guilds: sm.Guilds,
	}

	data, err := json.MarshalIndent(format, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(sm.filePath, data, 0644)
}

// GetGuildSettings サーバー設定を取得（存在しない場合はデフォルト）
func (sm *SettingsManager) GetGuildSettings(guildID string) GuildSettings {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	if settings, ok := sm.Guilds[guildID]; ok {
		return settings
	}

	return DefaultGuildSettings
}

// SetGuildSettings サーバー設定を保存
func (sm *SettingsManager) SetGuildSettings(guildID string, settings GuildSettings) error {
	sm.mu.Lock()
	sm.Guilds[guildID] = settings
	sm.mu.Unlock()

	return sm.Save()
}

// UpdateGuildSetting 特定の設定項目を更新
func (sm *SettingsManager) UpdateGuildSetting(guildID string, update func(*GuildSettings)) error {
	sm.mu.Lock()
	settings := sm.Guilds[guildID]
	if settings == (GuildSettings{}) {
		settings = DefaultGuildSettings
	}
	update(&settings)
	sm.Guilds[guildID] = settings
	sm.mu.Unlock()

	return sm.Save()
}
