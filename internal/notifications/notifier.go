package notifications

import (
"Koukyo_discord_bot/internal/config"
"Koukyo_discord_bot/internal/embeds"
"Koukyo_discord_bot/internal/monitor"
"fmt"
"log"
"sync"
"time"

"github.com/bwmarrin/discordgo"
)

// Tier 通知段階
type Tier int

const (
TierNone Tier = iota
Tier10       // 10%以上
Tier20       // 20%以上
Tier30       // 30%以上
Tier40       // 40%以上
Tier50       // 50%以上（メンション閾値）
)

// NotificationState サーバーごとの通知状態
type NotificationState struct {
LastTier          Tier
MentionTriggered  bool
PendingNotifyTask chan struct{} // 遅延通知のキャンセル用
WasZeroDiff       bool           // 前回が0%だったか
}

// Notifier 通知システム
type Notifier struct {
session  *discordgo.Session
monitor  *monitor.Monitor
settings *config.SettingsManager
states   map[string]*NotificationState
mu       sync.RWMutex
}

// NewNotifier 通知システムを作成
func NewNotifier(session *discordgo.Session, mon *monitor.Monitor, settings *config.SettingsManager) *Notifier {
return &Notifier{
session:  session,
monitor:  mon,
settings: settings,
states:   make(map[string]*NotificationState),
}
}

// getState サーバーの通知状態を取得
func (n *Notifier) getState(guildID string) *NotificationState {
n.mu.Lock()
defer n.mu.Unlock()

if state, ok := n.states[guildID]; ok {
return state
}

state := &NotificationState{
LastTier:          TierNone,
MentionTriggered:  false,
PendingNotifyTask: make(chan struct{}),
WasZeroDiff:       true, // 初回は0%とみなす
}
n.states[guildID] = state
return state
}

// CheckAndNotify 差分率をチェックして通知を送信
func (n *Notifier) CheckAndNotify(guildID string) {
settings := n.settings.GetGuildSettings(guildID)

// 自動通知が無効の場合はスキップ
if !settings.AutoNotifyEnabled {
return
}

// 通知チャンネルが設定されていない場合はスキップ
if settings.NotificationChannel == nil {
return
}

// 監視データを取得
data := n.monitor.GetLatestData()
if data == nil {
return
}

// 通知指標の値を取得
diffValue := getDiffValue(data, settings.NotificationMetric)

// 現在のTierを判定
currentTier := calculateTier(diffValue, settings.NotificationThreshold)
state := n.getState(guildID)

// 0%から変動した場合の通知（省電力モード解除）
if state.WasZeroDiff && diffValue > 0 {
n.sendZeroRecoveryNotification(guildID, settings, data, diffValue)
}

// 0%に戻った場合の通知（修復完了）
if !state.WasZeroDiff && diffValue == 0 {
n.sendZeroCompletionNotification(guildID, settings, data)
}

// Tierが変化した場合のみ通知
if currentTier > state.LastTier {
// 遅延通知を送信
n.scheduleDelayedNotification(guildID, settings, data, currentTier, diffValue)
}

// 状態を更新
state.LastTier = currentTier
state.MentionTriggered = diffValue >= settings.MentionThreshold
state.WasZeroDiff = (diffValue == 0)
}

// scheduleDelayedNotification 遅延通知をスケジュール
func (n *Notifier) scheduleDelayedNotification(
guildID string,
settings config.GuildSettings,
data *monitor.MonitorData,
tier Tier,
diffValue float64,
) {
state := n.getState(guildID)

// 既存の遅延通知をキャンセル
select {
case state.PendingNotifyTask <- struct{}{}:
default:
}

// 新しい遅延通知をスケジュール
go func() {
delay := time.Duration(settings.NotificationDelay * float64(time.Second))
select {
case <-time.After(delay):
// 遅延後に通知を送信
n.sendNotification(guildID, settings, data, tier, diffValue)
case <-state.PendingNotifyTask:
// キャンセルされた
log.Printf("Notification cancelled for guild %s", guildID)
}
}()
}

// sendNotification 通知を送信
func (n *Notifier) sendNotification(
guildID string,
settings config.GuildSettings,
data *monitor.MonitorData,
tier Tier,
diffValue float64,
) {
channelID := *settings.NotificationChannel

// メンション文字列を構築
mentionStr := ""
if diffValue >= settings.MentionThreshold && settings.MentionRole != nil {
mentionStr = fmt.Sprintf("<@&%s> ", *settings.MentionRole)
}

// メトリックラベル
metricLabel := "差分率"
if settings.NotificationMetric == "weighted" {
metricLabel = "加重差分率"
}

// 通知メッセージを作成
message := fmt.Sprintf(
"%s⚠️ **皇居が荒らされています！** %s: **%.2f%%**",
mentionStr,
metricLabel,
diffValue,
)

// Embedを作成
embed := &discordgo.MessageEmbed{
Title:       "🏯 Wplace 荒らし検知",
Description: fmt.Sprintf("現在の%s: **%.2f%%**", metricLabel, diffValue),
Color:       getTierColor(tier),
Fields: []*discordgo.MessageEmbedField{
{
Name:   "📊 差分率 (全体)",
Value:  fmt.Sprintf("%.2f%%", data.DiffPercentage),
Inline: true,
},
},
Timestamp: time.Now().Format(time.RFC3339),
Footer: &discordgo.MessageEmbedFooter{
Text: "自動通知システム",
},
}

// 加重差分率がある場合は追加
if data.WeightedDiffPercentage != nil {
embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
Name:   "📊 加重差分率 (菊重視)",
Value:  fmt.Sprintf("%.2f%%", *data.WeightedDiffPercentage),
Inline: true,
})
}

// 画像を取得して結合
var files []*discordgo.File
images := n.monitor.GetLatestImages()
if images != nil && images.LiveImage != nil && images.DiffImage != nil {
combinedImage, err := embeds.CombineImages(images.LiveImage, images.DiffImage)
if err == nil {
files = append(files, &discordgo.File{
Name:        "koukyo_status.png",
ContentType: "image/png",
Reader:      combinedImage,
})
embed.Image = &discordgo.MessageEmbedImage{
URL: "attachment://koukyo_status.png",
}
} else {
log.Printf("Failed to combine images for notification: %v", err)
}
}

// メッセージを送信
_, err := n.session.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
Content: message,
Embeds:  []*discordgo.MessageEmbed{embed},
Files:   files,
})

if err != nil {
log.Printf("Failed to send notification to channel %s: %v", channelID, err)
} else {
log.Printf("Notification sent to guild %s: %.2f%%", guildID, diffValue)
}
}

// sendZeroRecoveryNotification 0%からの回復通知を送信
func (n *Notifier) sendZeroRecoveryNotification(
guildID string,
settings config.GuildSettings,
data *monitor.MonitorData,
diffValue float64,
) {
channelID := *settings.NotificationChannel

// メトリックラベル
metricLabel := "差分率"
if settings.NotificationMetric == "weighted" {
metricLabel = "加重差分率"
}

// 通知メッセージを作成
message := fmt.Sprintf("🔔 **皇居に変化が検出されました** %s: **%.2f%%**", metricLabel, diffValue)

// Embedを作成
embed := &discordgo.MessageEmbed{
Title:       "🟢 Wplace 変化検知",
Description: fmt.Sprintf("完全な0%%から変動しました\n現在の%s: **%.2f%%**", metricLabel, diffValue),
Color:       0x00FF00, // 緑
Fields: []*discordgo.MessageEmbedField{
{
Name:   "📊 差分率 (全体)",
Value:  fmt.Sprintf("%.2f%%", data.DiffPercentage),
Inline: true,
},
},
Timestamp: time.Now().Format(time.RFC3339),
Footer: &discordgo.MessageEmbedFooter{
Text: "自動通知システム - 省電力モード解除",
},
}

// 加重差分率がある場合は追加
if data.WeightedDiffPercentage != nil {
embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
Name:   "📊 加重差分率 (菊重視)",
Value:  fmt.Sprintf("%.2f%%", *data.WeightedDiffPercentage),
Inline: true,
})
}

// 画像を取得して結合
var files []*discordgo.File
images := n.monitor.GetLatestImages()
if images != nil && images.LiveImage != nil && images.DiffImage != nil {
combinedImage, err := embeds.CombineImages(images.LiveImage, images.DiffImage)
if err == nil {
files = append(files, &discordgo.File{
Name:        "koukyo_status.png",
ContentType: "image/png",
Reader:      combinedImage,
})
embed.Image = &discordgo.MessageEmbedImage{
URL: "attachment://koukyo_status.png",
}
} else {
log.Printf("Failed to combine images for zero recovery notification: %v", err)
}
}

// メッセージを送信
_, err := n.session.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
Content: message,
Embeds:  []*discordgo.MessageEmbed{embed},
Files:   files,
})

if err != nil {
log.Printf("Failed to send zero recovery notification to channel %s: %v", channelID, err)
} else {
log.Printf("Zero recovery notification sent to guild %s: %.2f%%", guildID, diffValue)
}
}

// sendZeroCompletionNotification 0%に戻った時の通知を送信
func (n *Notifier) sendZeroCompletionNotification(
guildID string,
settings config.GuildSettings,
data *monitor.MonitorData,
) {
channelID := *settings.NotificationChannel

// メトリックラベル
metricLabel := "差分率"
if settings.NotificationMetric == "weighted" {
metricLabel = "加重差分率"
}

// 通知メッセージを作成
message := fmt.Sprintf("✅ **修復が完了しました！** %s: **0.00%%**", metricLabel)

// Embedを作成
embed := &discordgo.MessageEmbed{
Title:       "🎉 Wplace 修復完了",
Description: fmt.Sprintf("%sが0%%に戻りました\n# Pixel Perfect!", metricLabel),
Color:       0x00FF00, // 緑
Fields: []*discordgo.MessageEmbedField{
{
Name:   "📊 差分率 (全体)",
Value:  "0.00%",
Inline: true,
},
},
Timestamp: time.Now().Format(time.RFC3339),
Footer: &discordgo.MessageEmbedFooter{
Text: "自動通知システム - 修復完了",
},
}

// 加重差分率がある場合は追加
if data.WeightedDiffPercentage != nil {
embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
Name:   "📊 加重差分率 (菊重視)",
Value:  "0.00%",
Inline: true,
})
}

// 画像を取得して結合
var files []*discordgo.File
images := n.monitor.GetLatestImages()
if images != nil && images.LiveImage != nil && images.DiffImage != nil {
combinedImage, err := embeds.CombineImages(images.LiveImage, images.DiffImage)
if err == nil {
files = append(files, &discordgo.File{
Name:        "koukyo_status.png",
ContentType: "image/png",
Reader:      combinedImage,
})
embed.Image = &discordgo.MessageEmbedImage{
URL: "attachment://koukyo_status.png",
}
} else {
log.Printf("Failed to combine images for zero completion notification: %v", err)
}
}

// メッセージを送信
_, err := n.session.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
Content: message,
Embeds:  []*discordgo.MessageEmbed{embed},
Files:   files,
})

if err != nil {
log.Printf("Failed to send zero completion notification to channel %s: %v", channelID, err)
} else {
log.Printf("Zero completion notification sent to guild %s", guildID)
}
}
// ResetState サーバーの通知状態をリセット
func (n *Notifier) ResetState(guildID string) {
n.mu.Lock()
defer n.mu.Unlock()
delete(n.states, guildID)
}

// getDiffValue 指標に応じた差分値を取得
func getDiffValue(data *monitor.MonitorData, metric string) float64 {
if metric == "weighted" && data.WeightedDiffPercentage != nil {
return *data.WeightedDiffPercentage
}
return data.DiffPercentage
}

// calculateTier 差分率からTierを計算
func calculateTier(diffValue, threshold float64) Tier {
if diffValue < threshold {
return TierNone
}
if diffValue >= 50 {
return Tier50
}
if diffValue >= 40 {
return Tier40
}
if diffValue >= 30 {
return Tier30
}
if diffValue >= 20 {
return Tier20
}
return Tier10
}

// getTierColor Tierに応じた色を取得
func getTierColor(tier Tier) int {
switch tier {
case Tier50:
return 0xFF0000 // 赤
case Tier40:
return 0xFF4500 // オレンジレッド
case Tier30:
return 0xFFA500 // オレンジ
case Tier20:
return 0xFFD700 // ゴールド
case Tier10:
return 0xFFFF00 // 黄色
default:
return 0x808080 // グレー
}
}

// StartMonitoring 全サーバーの監視を開始
func (n *Notifier) StartMonitoring() {
go func() {
ticker := time.NewTicker(2 * time.Second)
defer ticker.Stop()

for range ticker.C {
// 監視データが更新されたら全サーバーをチェック
if !n.monitor.State.HasData() {
continue
}

// Botが参加している全サーバーをチェック
for _, guild := range n.session.State.Guilds {
guildID := guild.ID
n.CheckAndNotify(guildID)
}
}
}()

log.Println("Notification monitoring started")
}
