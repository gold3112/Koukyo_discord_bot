package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"Koukyo_discord_bot/internal/config"
	"Koukyo_discord_bot/internal/utils"
	"Koukyo_discord_bot/internal/wplace"

	"github.com/bwmarrin/discordgo"
	_ "golang.org/x/image/webp"
)

const (
	progressTargetsFileName  = "progress_targets.json"
	progressTargetsReloadTTL = 30 * time.Second
)

type progressTargetConfig struct {
	ID       string
	Label    string
	Origin   string
	Template string
	Interval time.Duration
}

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

type progressTargetResult struct {
	coord      *utils.Coordinate
	template   *watchTemplate
	diffPixels int
	progress   float64
	wplaceURL  string
	fullsize   string
	livePNG    []byte
	diffPNG    []byte
	mergedPNG  []byte
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
		ev := n.progressTargetsState.evaluateProgress(target.ID, guild.ID, result.progress)
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

func (n *Notifier) buildProgressTargetResult(target progressTargetConfig) (*progressTargetResult, error) {
	template, err := n.progressTargetsState.loadProgressTemplate(target.Template)
	if err != nil {
		return nil, err
	}
	coord, err := parseWatchOrigin(target.Origin)
	if err != nil {
		return nil, err
	}

	startTileX := coord.TileX + coord.PixelX/utils.WplaceTileSize
	startTileY := coord.TileY + coord.PixelY/utils.WplaceTileSize
	startPixelX := coord.PixelX % utils.WplaceTileSize
	startPixelY := coord.PixelY % utils.WplaceTileSize
	endPixelX := startPixelX + template.Width
	endPixelY := startPixelY + template.Height
	tilesX := (endPixelX + utils.WplaceTileSize - 1) / utils.WplaceTileSize
	tilesY := (endPixelY + utils.WplaceTileSize - 1) / utils.WplaceTileSize
	if startTileX < 0 || startTileY < 0 || startTileX+tilesX-1 >= utils.WplaceTilesPerEdge || startTileY+tilesY-1 >= utils.WplaceTilesPerEdge {
		return nil, fmt.Errorf("progress origin out of range: %s", target.Origin)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	tilesData, err := wplace.DownloadTilesGrid(ctx, nil, startTileX, startTileY, tilesX, tilesY, 16)
	cancel()
	if err != nil {
		return nil, fmt.Errorf("download failed: %w", err)
	}

	cropRect := image.Rect(startPixelX, startPixelY, startPixelX+template.Width, startPixelY+template.Height)
	liveImg, err := wplace.CombineTilesCroppedImage(tilesData, utils.WplaceTileSize, utils.WplaceTileSize, tilesX, tilesY, cropRect)
	if err != nil {
		return nil, fmt.Errorf("combine failed: %w", err)
	}

	maskedLive := applyTemplateAlphaMask(template.Img, liveImg)
	diffPixels, diffMask := buildDiffMask(template.Img, liveImg)
	progress := 0.0
	if template.OpaqueCount > 0 {
		progress = float64(template.OpaqueCount-diffPixels) * 100 / float64(template.OpaqueCount)
	}

	livePNG, err := encodePNG(maskedLive)
	if err != nil {
		return nil, err
	}
	diffPNG, err := encodePNG(diffMask)
	if err != nil {
		return nil, err
	}
	mergedPNG, err := buildCombinedPreview(livePNG, diffPNG)
	if err != nil {
		return nil, err
	}

	center := watchAreaCenter(coord, template.Width, template.Height)
	return &progressTargetResult{
		coord:      coord,
		template:   template,
		diffPixels: diffPixels,
		progress:   progress,
		wplaceURL:  utils.BuildWplaceURL(center.Lng, center.Lat, utils.ZoomFromImageSize(template.Width, template.Height)),
		fullsize:   fmt.Sprintf("%d-%d-%d-%d-%d-%d", coord.TileX, coord.TileY, coord.PixelX, coord.PixelY, template.Width, template.Height),
		livePNG:    livePNG,
		diffPNG:    diffPNG,
		mergedPNG:  mergedPNG,
	}, nil
}

func (n *Notifier) sendProgressNotification(
	channelID string,
	settings config.GuildSettings,
	target progressTargetConfig,
	result *progressTargetResult,
	isVandal bool,
	tier Tier,
) {
	title := "🎨 ピクセルアート進捗"
	colorCode := progressTierColor(tier)
	desc := fmt.Sprintf("制作進捗 **%.2f%%**", result.progress)
	if isVandal {
		title = "🚨 ピクセルアート荒らし検知"
		colorCode = getTierColor(tier)
		desc = fmt.Sprintf("制作進捗が **%.2f%%** に低下しました", result.progress)
	}
	embed := &discordgo.MessageEmbed{
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
				Value:  fmt.Sprintf("%.2f%%", result.progress),
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

	path := filepath.Join(w.dataDir, progressTargetsFileName)
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			w.configs = nil
			w.configsLoaded = time.Now()
			return nil, nil
		}
		return nil, err
	}
	cfgs, err := parseProgressTargetsConfig(raw)
	if err != nil {
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
	templatePath, err := resolveTemplatePath(w.dataDir, templateRef)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(templatePath)
	if err != nil {
		return nil, fmt.Errorf("template not found: %s", templateRef)
	}

	w.mu.Lock()
	if entry, ok := w.templateCache[templatePath]; ok && entry.ModTime.Equal(info.ModTime()) {
		w.mu.Unlock()
		return entry.Template, nil
	}
	w.mu.Unlock()

	f, err := os.Open(templatePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("failed to decode template: %s", templateRef)
	}
	nrgba := toNRGBAImage(img)
	opaque := countOpaque(nrgba)
	if opaque == 0 {
		return nil, fmt.Errorf("template has no opaque pixels: %s", templateRef)
	}

	t := &watchTemplate{
		Img:         nrgba,
		Width:       nrgba.Bounds().Dx(),
		Height:      nrgba.Bounds().Dy(),
		OpaqueCount: opaque,
	}
	w.mu.Lock()
	w.templateCache[templatePath] = &watchTemplateCacheEntry{
		Template: t,
		ModTime:  info.ModTime(),
	}
	w.mu.Unlock()
	return t, nil
}

func parseProgressTargetsConfig(raw []byte) ([]progressTargetConfig, error) {
	type rawTarget struct {
		ID              string `json:"id"`
		Label           string `json:"label"`
		Origin          string `json:"origin"`
		Template        string `json:"template"`
		TemplatePath    string `json:"template_path"`
		IntervalSeconds int    `json:"interval_seconds"`
		Interval        int    `json:"interval"`
	}

	build := func(id string, item rawTarget) (progressTargetConfig, error) {
		cfg := progressTargetConfig{
			ID:       strings.TrimSpace(item.ID),
			Label:    strings.TrimSpace(item.Label),
			Origin:   strings.TrimSpace(item.Origin),
			Template: strings.TrimSpace(item.Template),
		}
		if cfg.Template == "" {
			cfg.Template = strings.TrimSpace(item.TemplatePath)
		}
		if cfg.ID == "" {
			cfg.ID = strings.TrimSpace(id)
		}
		if cfg.ID == "" {
			cfg.ID = cfg.Label
		}
		if cfg.ID == "" {
			return progressTargetConfig{}, fmt.Errorf("progress target id is empty")
		}
		if cfg.Label == "" {
			cfg.Label = cfg.ID
		}
		if cfg.Origin == "" || cfg.Template == "" {
			return progressTargetConfig{}, fmt.Errorf("progress target %s missing origin/template", cfg.ID)
		}
		sec := item.IntervalSeconds
		if sec <= 0 {
			sec = item.Interval
		}
		if sec <= 0 {
			cfg.Interval = defaultWatchInterval
		} else {
			cfg.Interval = time.Duration(sec) * time.Second
		}
		return cfg, nil
	}

	var root struct {
		Targets []rawTarget `json:"targets"`
	}
	if err := json.Unmarshal(raw, &root); err == nil && len(root.Targets) > 0 {
		out := make([]progressTargetConfig, 0, len(root.Targets))
		for i, item := range root.Targets {
			cfg, err := build(strconv.Itoa(i), item)
			if err != nil {
				return nil, err
			}
			out = append(out, cfg)
		}
		return out, nil
	}

	var asMap map[string]rawTarget
	if err := json.Unmarshal(raw, &asMap); err == nil && len(asMap) > 0 {
		out := make([]progressTargetConfig, 0, len(asMap))
		for key, item := range asMap {
			cfg, err := build(key, item)
			if err != nil {
				return nil, err
			}
			out = append(out, cfg)
		}
		return out, nil
	}

	var asList []rawTarget
	if err := json.Unmarshal(raw, &asList); err == nil && len(asList) > 0 {
		out := make([]progressTargetConfig, 0, len(asList))
		for i, item := range asList {
			cfg, err := build(strconv.Itoa(i), item)
			if err != nil {
				return nil, err
			}
			out = append(out, cfg)
		}
		return out, nil
	}
	return nil, fmt.Errorf("progress_targets.json format is invalid")
}
