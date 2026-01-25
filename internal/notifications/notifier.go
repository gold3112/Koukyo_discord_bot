package notifications

import (
	"Koukyo_discord_bot/internal/activity"
	"Koukyo_discord_bot/internal/config"
	"Koukyo_discord_bot/internal/embeds"
	"Koukyo_discord_bot/internal/monitor"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
)

// NotificationState サーバーごとの通知状態
type NotificationState struct {
	LastTier          Tier
	MentionTriggered  bool
	PendingNotifyTask chan struct{} // 遅延通知のキャンセル用
	WasZeroDiff       bool          // 前回が0%だったか
}

// Notifier 通知システム
type Notifier struct {
	session                  *discordgo.Session
	monitor                  *monitor.Monitor
	settings                 *config.SettingsManager
	states                   map[string]*NotificationState
	mu                       sync.RWMutex
	lastTimelapseCompletedAt *time.Time
	lastPowerSaveMode        bool
	dataDir                  string
	lastDailyReportDate      string
	vandalUserNotifier       *VandalUserNotifier
	fixUserNotifier          *FixUserNotifier
}

// NewNotifier 通知システムを作成
func NewNotifier(session *discordgo.Session, mon *monitor.Monitor, settings *config.SettingsManager, dataDir string) *Notifier {
	return &Notifier{
		session:            session,
		monitor:            mon,
		settings:           settings,
		states:             make(map[string]*NotificationState),
		dataDir:            dataDir,
		vandalUserNotifier: NewVandalUserNotifier(session, settings),
		fixUserNotifier:    NewFixUserNotifier(session, settings),
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

	if n.monitor.State.PowerSaveMode {
		return
	}

	// 通知指標の値を取得
	diffValue := getDiffValue(data, settings.NotificationMetric)
	isZero := isZeroDiff(diffValue)

	// 現在のTierを判定
	currentTier := calculateTier(diffValue, settings.NotificationThreshold)
	state := n.getState(guildID)

	// 0%から変動した場合の通知（省電力モード解除）
	if state.WasZeroDiff && !isZero {
		n.sendZeroRecoveryNotification(guildID, settings, data, diffValue)
	}

	// 0%に戻った場合の通知（修復完了）
	if !state.WasZeroDiff && isZero {
		n.sendZeroCompletionNotification(guildID, settings, data)
	}

	// Tierが変化した場合のみ通知
	if currentTier != state.LastTier {
		// 遅延通知を送信
		if currentTier > state.LastTier {
			n.scheduleDelayedNotification(guildID, settings, data, currentTier, diffValue, notificationIncrease)
		} else {
			n.scheduleDelayedNotification(guildID, settings, data, currentTier, diffValue, notificationDecrease)
		}
	}

	// 状態を更新
	state.LastTier = currentTier
	state.MentionTriggered = diffValue >= settings.MentionThreshold
	state.WasZeroDiff = isZero
}

// scheduleDelayedNotification 遅延通知をスケジュール
func (n *Notifier) scheduleDelayedNotification(
	guildID string,
	settings config.GuildSettings,
	data *monitor.MonitorData,
	tier Tier,
	diffValue float64,
	kind notificationKind,
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
			if kind == notificationDecrease {
				n.sendDecreaseNotification(guildID, settings, data, tier, diffValue)
				return
			}
			n.sendNotification(guildID, settings, data, tier, diffValue)
		case <-state.PendingNotifyTask:
			// キャンセルされた
			log.Printf("Notification cancelled for guild %s", guildID)
		}
	}()
}

type notificationKind int

const (
	notificationIncrease notificationKind = iota
	notificationDecrease
)

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

	// Tier に応じた通知メッセージを構築
	var tierDesc string
	switch tier {
	case Tier50:
		tierDesc = "50%以上に急増"
	case Tier40:
		tierDesc = "40%台に増加"
	case Tier30:
		tierDesc = "30%台に増加"
	case Tier20:
		tierDesc = "20%台に増加"
	case Tier10:
		tierDesc = "10%台に増加"
	default:
		tierDesc = "変動"
	}

	// 通知メッセージを作成（新フォーマット）
	message := fmt.Sprintf(
		"%s【Wplace速報】 🚨 %sが%sしました！[現在%.2f%%]",
		mentionStr,
		metricLabel,
		tierDesc,
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
			{
				Name:   "📈 差分ピクセル (全体)",
				Value:  fmt.Sprintf("%d / %d", data.DiffPixels, data.TotalPixels),
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
			Name:   "🔍 加重差分率 (菊重視)",
			Value:  fmt.Sprintf("%.2f%%", *data.WeightedDiffPercentage),
			Inline: true,
		})
	}

	// 加重差分ピクセルがある場合は追加
	if data.ChrysanthemumDiffPixels > 0 || data.BackgroundDiffPixels > 0 {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   "🔍 差分ピクセル (菊/背景)",
			Value:  fmt.Sprintf("菊 %d / %d | 背景 %d / %d", data.ChrysanthemumDiffPixels, data.ChrysanthemumTotalPixels, data.BackgroundDiffPixels, data.BackgroundTotalPixels),
			Inline: false,
		})
	}

	// 監視ピクセル数を追加
	embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
		Name:   "📐 監視ピクセル数",
		Value:  fmt.Sprintf("全体 %d | 菊 %d | 背景 %d", data.TotalPixels, data.ChrysanthemumTotalPixels, data.BackgroundTotalPixels),
		Inline: false,
	})

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

// sendDecreaseNotification Tierが下がった通知を送信
func (n *Notifier) sendDecreaseNotification(
	guildID string,
	settings config.GuildSettings,
	data *monitor.MonitorData,
	tier Tier,
	diffValue float64,
) {
	channelID := *settings.NotificationChannel

	// メトリックラベル
	metricLabel := "差分率"
	if settings.NotificationMetric == "weighted" {
		metricLabel = "加重差分率"
	}

	tierLabel := tierRangeLabel(tier, settings.NotificationThreshold)
	message := fmt.Sprintf(
		"【Wplace速報】 %sが%sまで減少しました。[現在%.2f%%]",
		metricLabel,
		tierLabel,
		diffValue,
	)

	embed := &discordgo.MessageEmbed{
		Title:       "🏯 Wplace 差分減少",
		Description: fmt.Sprintf("現在の%s: **%.2f%%**", metricLabel, diffValue),
		Color:       getTierColor(tier),
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:   "📊 差分率 (全体)",
				Value:  fmt.Sprintf("%.2f%%", data.DiffPercentage),
				Inline: true,
			},
			{
				Name:   "📈 差分ピクセル (全体)",
				Value:  fmt.Sprintf("%d / %d", data.DiffPixels, data.TotalPixels),
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
			Name:   "🔍 加重差分率 (菊重視)",
			Value:  fmt.Sprintf("%.2f%%", *data.WeightedDiffPercentage),
			Inline: true,
		})
	}

	// 加重差分ピクセルがある場合は追加
	if data.ChrysanthemumDiffPixels > 0 || data.BackgroundDiffPixels > 0 {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   "🔍 差分ピクセル (菊/背景)",
			Value:  fmt.Sprintf("菊 %d / %d | 背景 %d / %d", data.ChrysanthemumDiffPixels, data.ChrysanthemumTotalPixels, data.BackgroundDiffPixels, data.BackgroundTotalPixels),
			Inline: false,
		})
	}

	// 監視ピクセル数を追加
	embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
		Name:   "📐 監視ピクセル数",
		Value:  fmt.Sprintf("全体 %d | 菊 %d | 背景 %d", data.TotalPixels, data.ChrysanthemumTotalPixels, data.BackgroundTotalPixels),
		Inline: false,
	})

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
			log.Printf("Failed to combine images for decrease notification: %v", err)
		}
	}

	// メッセージを送信
	_, err := n.session.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
		Content: message,
		Embeds:  []*discordgo.MessageEmbed{embed},
		Files:   files,
	})

	if err != nil {
		log.Printf("Failed to send decrease notification to channel %s: %v", channelID, err)
	} else {
		log.Printf("Decrease notification sent to guild %s: %.2f%%", guildID, diffValue)
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
	message := fmt.Sprintf("🔔 【Wplace速報】変化検知 %s: **%.2f%%**に上昇", metricLabel, diffValue)

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
			{
				Name:   "📈 差分ピクセル (全体)",
				Value:  fmt.Sprintf("%d / %d", data.DiffPixels, data.TotalPixels),
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
			Name:   "🔍 加重差分率 (菊重視)",
			Value:  fmt.Sprintf("%.2f%%", *data.WeightedDiffPercentage),
			Inline: true,
		})
	}

	// 加重差分ピクセルがある場合は追加
	if data.ChrysanthemumDiffPixels > 0 || data.BackgroundDiffPixels > 0 {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   "🔍 差分ピクセル (菊/背景)",
			Value:  fmt.Sprintf("菊 %d / %d | 背景 %d / %d", data.ChrysanthemumDiffPixels, data.ChrysanthemumTotalPixels, data.BackgroundDiffPixels, data.BackgroundTotalPixels),
			Inline: false,
		})
	}

	// 監視ピクセル数を追加
	embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
		Name:   "📐 監視ピクセル数",
		Value:  fmt.Sprintf("全体 %d | 菊 %d | 背景 %d", data.TotalPixels, data.ChrysanthemumTotalPixels, data.BackgroundTotalPixels),
		Inline: false,
	})

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
	message := fmt.Sprintf("✅ 【Wplace速報】修復完了！ %s: **0.00%%** # Pixel Perfect!", metricLabel)

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
			{
				Name:   "📈 差分ピクセル (全体)",
				Value:  fmt.Sprintf("0 / %d", data.TotalPixels),
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
			Name:   "🔍 加重差分率 (菊重視)",
			Value:  "0.00%",
			Inline: true,
		})
	}

	// 監視ピクセル数を追加
	embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
		Name:   "📐 監視ピクセル数",
		Value:  fmt.Sprintf("全体 %d | 菊 %d | 背景 %d", data.TotalPixels, data.ChrysanthemumTotalPixels, data.BackgroundTotalPixels),
		Inline: false,
	})

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

func (n *Notifier) NotifyNewUser(kind string, user activity.UserActivity) {
	switch kind {
	case "vandal":
		if n.vandalUserNotifier != nil {
			n.vandalUserNotifier.Notify(user)
		}
	case "fix":
		if n.fixUserNotifier != nil {
			n.fixUserNotifier.Notify(user)
		}
	}
}

