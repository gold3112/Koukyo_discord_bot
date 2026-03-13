package config

import (
	"Koukyo_discord_bot/internal/utils"
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
	AchievementChannel        *string `json:"achievement_channel,omitempty"`         // 実績通知チャンネル
	ProgressChannel           *string `json:"progress_channel,omitempty"`            // 進捗通知チャンネル
	AutoNotifyEnabled         bool    `json:"auto_notify_enabled"`                   // 自動通知ON/OFF
	ProgressNotifyEnabled     bool    `json:"progress_notify_enabled"`               // 進捗通知ON/OFF
	NotificationThreshold     float64 `json:"notification_threshold"`                // 通知閾値（%）
	MentionRole               *string `json:"mention_role,omitempty"`                // メンションロールID
	MentionThreshold          float64 `json:"mention_threshold"`                     // メンション閾値（%）
	NotificationMetric        string  `json:"notification_metric"`                   // 通知指標: "overall" or "weighted"
}

// DefaultGuildSettings デフォルト設定
var DefaultGuildSettings = GuildSettings{
	AutoNotifyEnabled:     true,
	ProgressNotifyEnabled: false,
	NotificationThreshold: 10.0,
	MentionThreshold:      50.0,
	NotificationMetric:    "overall",
}

// SettingsManager 設定管理
type SettingsManager struct {
	filePath      string
	settings      map[string]GuildSettings
	userDMEnabled map[string]bool
	userDMPath    string
	mu            sync.RWMutex
	dirty         bool
	userDMDirty   bool
	shutdownCh    chan struct{}
	closeOnce     sync.Once
	saverDone     chan struct{}
}

// NewSettingsManager 設定マネージャーを作成
func NewSettingsManager(configPath string) *SettingsManager {
	sm := &SettingsManager{
		settings:      make(map[string]GuildSettings),
		userDMEnabled: make(map[string]bool),
		filePath:      configPath,
		userDMPath:    filepath.Join(filepath.Dir(configPath), "user_dm.json"),
		shutdownCh:    make(chan struct{}),
		saverDone:     make(chan struct{}),
	}
	if err := sm.load(); err != nil {
		// log.Printf("Failed to load settings, starting with default: %v", err)
	}
	go sm.periodicSaver(30 * time.Second)
	return sm
}

// Close シャットダウン処理
func (sm *SettingsManager) Close() {
	sm.closeOnce.Do(func() {
		close(sm.shutdownCh)
		<-sm.saverDone
	})
}

func (sm *SettingsManager) periodicSaver(interval time.Duration) {
	defer close(sm.saverDone)
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
	sm.loadUserDMUnsafe()
	return nil
}

// SaveIfDirty 変更があれば設定をファイルに保存
func (sm *SettingsManager) SaveIfDirty() error {
	sm.mu.RLock()
	dirty := sm.dirty
	dmDirty := sm.userDMDirty
	sm.mu.RUnlock()

	if !dirty && !dmDirty {
		return nil
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	var firstErr error
	if sm.dirty {
		if err := sm.saveUnsafe(); err == nil {
			sm.dirty = false
		} else {
			firstErr = err
		}
	}
	if sm.userDMDirty {
		if err := sm.saveUserDMUnsafe(); err == nil {
			sm.userDMDirty = false
		} else if firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (sm *SettingsManager) saveUnsafe() error {
	data, err := json.MarshalIndent(sm.settings, "", "  ")
	if err != nil {
		return err
	}

	return utils.WriteFileAtomic(sm.filePath, data)
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

// loadUserDMUnsafe DMユーザー設定をファイルから読み込む（mu保持中に呼ぶこと）
func (sm *SettingsManager) loadUserDMUnsafe() {
	if sm.userDMPath == "" {
		return
	}
	data, err := os.ReadFile(sm.userDMPath)
	if err != nil || len(data) == 0 {
		return
	}
	var m map[string]bool
	if err := json.Unmarshal(data, &m); err == nil && m != nil {
		sm.userDMEnabled = m
	}
}

// saveUserDMUnsafe DMユーザー設定をファイルに書き込む（mu保持中に呼ぶこと）
func (sm *SettingsManager) saveUserDMUnsafe() error {
	data, err := json.MarshalIndent(sm.userDMEnabled, "", "  ")
	if err != nil {
		return err
	}
	return utils.WriteFileAtomic(sm.userDMPath, data)
}

// GetUserDMEnabled ユーザーのDM通知設定を取得
func (sm *SettingsManager) GetUserDMEnabled(userID string) bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.userDMEnabled[userID]
}

// SetUserDMEnabled ユーザーのDM通知設定を更新
func (sm *SettingsManager) SetUserDMEnabled(userID string, enabled bool) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if enabled {
		sm.userDMEnabled[userID] = true
	} else {
		delete(sm.userDMEnabled, userID)
	}
	sm.userDMDirty = true
}

// GetDMEnabledUserIDs DM通知が有効なユーザーIDの一覧を返す
func (sm *SettingsManager) GetDMEnabledUserIDs() []string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	ids := make([]string, 0, len(sm.userDMEnabled))
	for id, enabled := range sm.userDMEnabled {
		if enabled {
			ids = append(ids, id)
		}
	}
	return ids
}
