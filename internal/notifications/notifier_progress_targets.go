package notifications

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"Koukyo_discord_bot/internal/config"
	"github.com/bwmarrin/discordgo"
	_ "golang.org/x/image/webp"
)

const (
	progressTargetsFileName  = "progress_targets.json"
	progressTargetsReloadTTL = 30 * time.Second
)

type progressTargetConfig = commonTargetConfig

type progressTargetStatus struct {
	NextRun     time.Time
	Running     bool
	GuildStates map[string]*progressNotificationState
}

type progressNotificationState struct {
	LastTier    Tier
	LastPercent float64
	HasValue    bool
}

type progressTargetsRuntime struct {
	dataDir string

	mu            sync.Mutex
	configs       []progressTargetConfig
	configsLoaded time.Time
	statuses      map[string]*progressTargetStatus
	errorNotified map[string]bool
	templateCache map[string]*watchTemplateCacheEntry
}

type progressTargetEval struct {
	increase bool
	decrease bool
	tier     Tier
}

func newProgressTargetsRuntime(dataDir string) *progressTargetsRuntime {
	return &progressTargetsRuntime{
		dataDir:       dataDir,
		statuses:      make(map[string]*progressTargetStatus),
		errorNotified: make(map[string]bool),
		templateCache: make(map[string]*watchTemplateCacheEntry),
	}
}

func (n *Notifier) startProgressTargetsLoop() {
	if n.progressTargetsState == nil || n.dataDir == "" {
		return
	}
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		sem := make(chan struct{}, maxWatchParallel)
		for range ticker.C {
			targets, err := n.progressTargetsState.loadProgressConfigs()
			if err != nil {
				log.Printf("progress_targets: failed to load config: %v", err)
				continue
			}
			now := time.Now()
			for _, target := range targets {
				if !n.progressTargetsState.tryStartProgress(target.ID, now, target.Interval) {
					continue
				}
				sem <- struct{}{}
				go func(cfg progressTargetConfig) {
					defer func() {
						<-sem
						n.progressTargetsState.finishProgress(cfg.ID)
					}()
					n.runProgressTarget(cfg)
				}(target)
			}
		}
	}()
}

// HandleProgressTargetManual triggers a one-off fetch for a target id and posts to the channel.
func (n *Notifier) HandleProgressTargetManual(channelID, targetID string) bool {
	if n == nil || n.progressTargetsState == nil {
		return false
	}
	target, ok := n.progressTargetsState.findTargetByID(targetID)
	if !ok {
		return false
	}
	go func() {
		result, err := n.buildProgressTargetResult(target)
		if err != nil {
			_, _ = n.session.ChannelMessageSend(channelID, fmt.Sprintf("❌ 進捗監視の取得に失敗しました: %v", err))
			return
		}
		n.sendProgressManual(channelID, target, result)
	}()
	return true
}

func (n *Notifier) runProgressTarget(target progressTargetConfig) {
	result, err := n.buildProgressTargetResult(target)
	if err != nil {
		n.handleProgressTargetError(target, err, true)
		return
	}
	for _, guild := range n.session.State.Guilds {
		settings := n.settings.GetGuildSettings(guild.ID)
		if !settings.ProgressNotifyEnabled || settings.ProgressChannel == nil {
			continue
		}
		ev := n.progressTargetsState.evaluateProgress(target.ID, guild.ID, result.progressPercent)
		if !ev.increase && !ev.decrease {
			continue
		}
		if ev.increase {
			n.sendProgressNotification(*settings.ProgressChannel, settings, target, result, false, ev.tier)
		}
		if ev.decrease {
			n.sendProgressNotification(*settings.ProgressChannel, settings, target, result, true, ev.tier)
		}
	}
	n.progressTargetsState.clearProgressErrorNotified(target.ID)
}

func (n *Notifier) buildProgressTargetResult(target progressTargetConfig) (*targetResult, error) {
	template, err := n.progressTargetsState.loadProgressTemplate(target.Template)
	if err != nil {
		return nil, err
	}
	coord, err := parseWatchOrigin(target.Origin)
	if err != nil {
		return nil, err
	}
	return buildTargetResult(coord, template)
}

func (n *Notifier) sendProgressNotification(
	channelID string,
	settings config.GuildSettings,
	target progressTargetConfig,
	result *targetResult,
	isVandal bool,
	tier Tier,
) {
	embed := n.buildProgressEmbed("🎨 ピクセルアート進捗", target, result, isVandal, tier)

	_, err := n.session.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
		Embeds: []*discordgo.MessageEmbed{embed},
		Files: []*discordgo.File{
			{
				Name:        "progress_preview.png",
				ContentType: "image/png",
				Reader:      bytes.NewReader(result.mergedPNG),
			},
		},
	})
	if err != nil {
		log.Printf("progress_targets: notify failed channel=%s target=%s err=%v", channelID, target.ID, err)
	}
}

func (n *Notifier) buildProgressEmbed(title string, target progressTargetConfig, result *targetResult, isVandal bool, tier Tier) *discordgo.MessageEmbed {
	colorCode := progressTierColor(tier)
	desc := fmt.Sprintf("制作進捗 **%.2f%%**", result.progressPercent)
	if isVandal {
		title = "🚨 ピクセルアート荒らし検知"
		colorCode = getTierColor(tier)
		desc = fmt.Sprintf("制作進捗が **%.2f%%** に低下しました", result.progressPercent)
	}
	return &discordgo.MessageEmbed{
		Title:       title,
		Description: desc,
		Color:       colorCode,
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:   "ID",
				Value:  fmt.Sprintf("`%s`", target.ID),
				Inline: true,
			},
			{
				Name:   "進捗率",
				Value:  fmt.Sprintf("%.2f%%", result.progressPercent),
				Inline: true,
			},
			{
				Name:   "未一致ピクセル",
				Value:  fmt.Sprintf("%d / %d", result.diffPixels, result.template.OpaqueCount),
				Inline: true,
			},
			{
				Name:   "対象",
				Value:  fmt.Sprintf("`%s`", target.Label),
				Inline: true,
			},
			{
				Name:   "左上座標",
				Value:  fmt.Sprintf("`%s`", target.Origin),
				Inline: true,
			},
			{
				Name:   "監視サイズ",
				Value:  fmt.Sprintf("`%dx%d`", result.template.Width, result.template.Height),
				Inline: true,
			},
			{
				Name:   "手動取得",
				Value:  formatManualCommands(target.ID, target.Aliases),
				Inline: true,
			},
			{
				Name:   "Wplace.live",
				Value:  fmt.Sprintf("[地図で見る](%s)\n`/get fullsize:%s`", result.wplaceURL, result.fullsize),
				Inline: false,
			},
		},
		Image: &discordgo.MessageEmbedImage{
			URL: "attachment://progress_preview.png",
		},
		Timestamp: time.Now().Format(time.RFC3339),
	}
}

func (n *Notifier) sendProgressManual(channelID string, target progressTargetConfig, result *targetResult) {
	embed := n.buildProgressEmbed("📌 ピクセルアート進捗 (手動取得)", target, result, false, TierNone)
	_, err := n.session.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
		Embeds: []*discordgo.MessageEmbed{embed},
		Files: []*discordgo.File{
			{
				Name:        "progress_preview.png",
				ContentType: "image/png",
				Reader:      bytes.NewReader(result.mergedPNG),
			},
		},
	})
	if err != nil {
		log.Printf("progress_targets: manual notify failed channel=%s target=%s err=%v", channelID, target.ID, err)
	}
}

func progressTierColor(tier Tier) int {
	switch tier {
	case Tier90, Tier100, Tier80:
		return 0xF1C40F // gold
	case Tier60, Tier70, Tier50, Tier40:
		return 0x2ECC71 // green
	case Tier30, Tier20, Tier10:
		return 0x3498DB // blue
	default:
		return 0x3498DB
	}
}

func (n *Notifier) handleProgressTargetError(target progressTargetConfig, err error, notifyOnce bool) {
	log.Printf("progress_targets: target=%s error=%v", target.ID, err)
	if !notifyOnce {
		return
	}
	if !n.progressTargetsState.shouldNotifyProgressError(target.ID) {
		return
	}
	n.sendProgressTargetErrorNotification(target, err)
}

func (n *Notifier) sendProgressTargetErrorNotification(target progressTargetConfig, err error) {
	for _, guild := range n.session.State.Guilds {
		settings := n.settings.GetGuildSettings(guild.ID)
		if !settings.ProgressNotifyEnabled || settings.ProgressChannel == nil {
			continue
		}
		embed := &discordgo.MessageEmbed{
			Title:       "⚠️ 進捗監視エラー",
			Description: fmt.Sprintf("対象 `%s` をスキップします。", target.Label),
			Color:       0xF39C12,
			Fields: []*discordgo.MessageEmbedField{
				{
					Name:   "対象",
					Value:  fmt.Sprintf("`%s`", target.Label),
					Inline: true,
				},
				{
					Name:   "原因",
					Value:  fmt.Sprintf("`%v`", err),
					Inline: false,
				},
			},
			Timestamp: time.Now().Format(time.RFC3339),
		}
		if _, sendErr := n.session.ChannelMessageSendEmbed(*settings.ProgressChannel, embed); sendErr != nil {
			log.Printf("progress_targets: error notify failed guild=%s target=%s err=%v", guild.ID, target.ID, sendErr)
		}
	}
}

func (w *progressTargetsRuntime) loadProgressConfigs() ([]progressTargetConfig, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.configs != nil && time.Since(w.configsLoaded) < progressTargetsReloadTTL {
		return cloneProgressConfigs(w.configs), nil
	}

	path := targetConfigPath(w.dataDir, progressTargetsFileName)
	cfgs, err := loadTargetConfigs(path, defaultWatchInterval)
	if err != nil {
		if os.IsNotExist(err) {
			w.configs = nil
			w.configsLoaded = time.Now()
			return nil, nil
		}
		return nil, err
	}
	w.configs = cfgs
	w.configsLoaded = time.Now()
	return cloneProgressConfigs(w.configs), nil
}

func cloneProgressConfigs(src []progressTargetConfig) []progressTargetConfig {
	if src == nil {
		return nil
	}
	out := make([]progressTargetConfig, len(src))
	copy(out, src)
	return out
}

func (w *progressTargetsRuntime) tryStartProgress(targetID string, now time.Time, interval time.Duration) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	st, ok := w.statuses[targetID]
	if !ok {
		st = &progressTargetStatus{
			GuildStates: make(map[string]*progressNotificationState),
		}
		w.statuses[targetID] = st
	}
	if st.Running {
		return false
	}
	if !st.NextRun.IsZero() && st.NextRun.After(now) {
		return false
	}
	if interval <= 0 {
		interval = defaultWatchInterval
	}
	st.Running = true
	st.NextRun = now.Add(interval)
	return true
}

func (w *progressTargetsRuntime) finishProgress(targetID string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if st, ok := w.statuses[targetID]; ok {
		st.Running = false
	}
}

func (w *progressTargetsRuntime) evaluateProgress(targetID, guildID string, progress float64) progressTargetEval {
	w.mu.Lock()
	defer w.mu.Unlock()

	st, ok := w.statuses[targetID]
	if !ok {
		st = &progressTargetStatus{
			GuildStates: make(map[string]*progressNotificationState),
		}
		w.statuses[targetID] = st
	}
	if st.GuildStates == nil {
		st.GuildStates = make(map[string]*progressNotificationState)
	}

	gs, ok := st.GuildStates[guildID]
	if !ok {
		gs = &progressNotificationState{
			LastTier: TierNone,
		}
		st.GuildStates[guildID] = gs
	}

	currentTier := calculateTier(progress, 10.0)
	ev := progressTargetEval{tier: currentTier}
	if gs.HasValue {
		if currentTier > gs.LastTier {
			ev.increase = true
		} else if currentTier < gs.LastTier {
			ev.decrease = true
		}
	} else if currentTier > TierNone {
		ev.increase = true
	}
	gs.LastTier = currentTier
	gs.LastPercent = progress
	gs.HasValue = true
	return ev
}

func (w *progressTargetsRuntime) shouldNotifyProgressError(targetID string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.errorNotified[targetID] {
		return false
	}
	w.errorNotified[targetID] = true
	return true
}

func (w *progressTargetsRuntime) clearProgressErrorNotified(targetID string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.errorNotified, targetID)
}

func (w *progressTargetsRuntime) loadProgressTemplate(templateRef string) (*watchTemplate, error) {
	return loadTemplateCached(&w.mu, w.templateCache, w.dataDir, templateRef)
}

func (w *progressTargetsRuntime) findTargetByID(targetID string) (progressTargetConfig, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.configs == nil {
		return progressTargetConfig{}, false
	}
	for _, cfg := range w.configs {
		if targetIDMatches(cfg, targetID) {
			return cfg, true
		}
	}
	return progressTargetConfig{}, false
}
