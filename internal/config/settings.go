package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// GuildSettings サーバーごとの設定
type GuildSettings struct {
	NotificationChannel       *string `json:"notification_channel,omitempty"`        // 通知チャンネルID
	NotificationVandalChannel *string `json:"notification_vandal_channel,omitempty"` // 荒らしユーザー通知チャンネル
	NotificationFixChannel    *string `json:"notification_fix_channel,omitempty"`    // 修復ユーザー通知チャンネル
	AutoNotifyEnabled         bool    `json:"auto_notify_enabled"`                   // 自動通知ON/OFF
	NotificationDelay         float64 `json:"notification_delay"`                    // 通知遅延（秒）
	NotificationThreshold     float64 `json:"notification_threshold"`                // 通知閾値（%）
	MentionRole               *string `json:"mention_role,omitempty"`                // メンションロールID
	MentionThreshold          float64 `json:"mention_threshold"`                     // メンション閾値（%）
	NotificationMetric        string  `json:"notification_metric"`                   // 通知指標: "overall" or "weighted"
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
	filePath   string
	settings   map[string]GuildSettings
	mu         sync.RWMutex
	dirty      bool
	shutdownCh chan struct{}
}

// NewSettingsManager 設定マネージャーを作成
func NewSettingsManager(configPath string) *SettingsManager {
	sm := &SettingsManager{
		settings:   make(map[string]GuildSettings),
		filePath:   configPath,
		shutdownCh: make(chan struct{}),
	}
	if err := sm.load(); err != nil {
		// log.Printf("Failed to load settings, starting with default: %v", err)
	}
	go sm.periodicSaver(30 * time.Second)
	return sm
}

// Close シャットダウン処理
func (sm *SettingsManager) Close() {
	close(sm.shutdownCh)
}

func (sm *SettingsManager) periodicSaver(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			sm.SaveIfDirty()
		case <-sm.shutdownCh:
			sm.SaveIfDirty()
			return
		}
	}
}

// load 設定をファイルから読み込む（ロックは呼び出し元が管理）
func (sm *SettingsManager) load() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if _, err := os.Stat(sm.filePath); os.IsNotExist(err) {
		return nil // 新規作成
	}

	data, err := os.ReadFile(sm.filePath)
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return nil // 空ファイル
	}

	if err := json.Unmarshal(data, &sm.settings); err != nil {
		return err
	}
	if sm.settings == nil {
		sm.settings = make(map[string]GuildSettings)
	}
	return nil
}

// SaveIfDirty 変更があれば設定をファイルに保存
func (sm *SettingsManager) SaveIfDirty() error {
	sm.mu.RLock()
	if !sm.dirty {
		sm.mu.RUnlock()
		return nil
	}
	sm.mu.RUnlock()

	sm.mu.Lock()
	defer sm.mu.Unlock()

	// ダブルチェック
	if !sm.dirty {
		return nil
	}

	err := sm.saveUnsafe()
	if err == nil {
		sm.dirty = false
	}
	return err
}

func (sm *SettingsManager) saveUnsafe() error {
	dir := filepath.Dir(sm.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(sm.settings, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(sm.filePath, data, 0644)
}

// GetGuildSettings サーバー設定を取得（存在しない場合はデフォルト）
func (sm *SettingsManager) GetGuildSettings(guildID string) GuildSettings {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	if settings, ok := sm.settings[guildID]; ok {
		return settings
	}

	return DefaultGuildSettings
}

// SetGuildSettings サーバー設定を保存
func (sm *SettingsManager) SetGuildSettings(guildID string, settings GuildSettings) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.settings[guildID] = settings
	sm.dirty = true
}

// UpdateGuildSetting 特定の設定項目を更新
func (sm *SettingsManager) UpdateGuildSetting(guildID string, update func(*GuildSettings)) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	settings, ok := sm.settings[guildID]
	if !ok {
		settings = DefaultGuildSettings
	}
	update(&settings)
	sm.settings[guildID] = settings
	sm.dirty = true
}
