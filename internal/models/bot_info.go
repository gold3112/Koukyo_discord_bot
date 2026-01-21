package models

import "time"

// BotInfo Botの情報を保持
type BotInfo struct {
	Version   string
	StartTime time.Time
}

// NewBotInfo 新しいBotInfo構造体を作成
func NewBotInfo(version string) *BotInfo {
	return &BotInfo{
		Version:   version,
		StartTime: time.Now(),
	}
}

// Uptime Bot起動からの経過時間を返す
func (b *BotInfo) Uptime() time.Duration {
	return time.Since(b.StartTime)
}
