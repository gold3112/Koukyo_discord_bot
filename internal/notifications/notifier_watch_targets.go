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
	watchTargetsFileName  = "watch_targets.json"
	templateImageDirName  = "template_img"
	watchTargetsReloadTTL = 30 * time.Second
	defaultWatchInterval  = 30 * time.Second
	maxWatchParallel      = 2
)

type watchTargetConfig struct {
	ID       string
	Label    string
	Origin   string
	Template string
	Interval time.Duration
}

type watchTargetStatus struct {
	NextRun     time.Time
	Running     bool
	GuildStates map[string]*NotificationState
}

type watchTemplate struct {
	Img         *image.NRGBA
	Width       int
	Height      int
	OpaqueCount int
}

type watchTemplateCacheEntry struct {
	Template *watchTemplate
	ModTime  time.Time
}

type watchTargetsRuntime struct {
	dataDir string

	mu            sync.Mutex
	configs       []watchTargetConfig
	configsLoaded time.Time
	statuses      map[string]*watchTargetStatus
	errorNotified map[string]bool
	templateCache map[string]*watchTemplateCacheEntry
}

type watchTargetResult struct {
	coord      *utils.Coordinate
	template   *watchTemplate
	diffPixels int
	percent    float64
	wplaceURL  string
	fullsize   string
	livePNG    []byte
	diffPNG    []byte
	mergedPNG  []byte
}

type watchTargetEval struct {
	sendIncrease bool
	sendDecrease bool
	sendRecover  bool
	sendComplete bool
	tier         Tier
}

func newWatchTargetsRuntime(dataDir string) *watchTargetsRuntime {
	return &watchTargetsRuntime{
		dataDir:       dataDir,
		statuses:      make(map[string]*watchTargetStatus),
		errorNotified: make(map[string]bool),
		templateCache: make(map[string]*watchTemplateCacheEntry),
	}
}

func (n *Notifier) startWatchTargetsLoop() {
	if n.watchTargetsState == nil || n.dataDir == "" {
		return
	}
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		sem := make(chan struct{}, maxWatchParallel)
		for range ticker.C {
			targets, err := n.watchTargetsState.loadConfigs()
			if err != nil {
				log.Printf("watch_targets: failed to load config: %v", err)
				continue
			}
			now := time.Now()
			for _, target := range targets {
				if !n.watchTargetsState.tryStart(target.ID, now, target.Interval) {
					continue
				}
				sem <- struct{}{}
				go func(cfg watchTargetConfig) {
					defer func() {
						<-sem
						n.watchTargetsState.finish(cfg.ID)
					}()
					n.runWatchTarget(cfg)
				}(target)
			}
		}
	}()
}

// HandleWatchTargetManual triggers a one-off fetch for a target id and posts to the channel.
func (n *Notifier) HandleWatchTargetManual(channelID, targetID string) bool {
	if n == nil || n.watchTargetsState == nil {
		return false
	}
	target, ok := n.watchTargetsState.findTargetByID(targetID)
	if !ok {
		return false
	}
	go func() {
		result, err := n.buildWatchTargetResult(target)
		if err != nil {
			_, _ = n.session.ChannelMessageSend(channelID, fmt.Sprintf("❌ 追加監視の取得に失敗しました: %v", err))
			return
		}
		n.sendWatchTargetManual(channelID, target, result)
	}()
	return true
}

func (n *Notifier) runWatchTarget(target watchTargetConfig) {
	result, err := n.buildWatchTargetResult(target)
	if err != nil {
		n.handleWatchTargetError(target, err, true)
		return
	}
	for _, guild := range n.session.State.Guilds {
		settings := n.settings.GetGuildSettings(guild.ID)
		if !settings.AutoNotifyEnabled || settings.NotificationChannel == nil {
			continue
		}
		eval := n.watchTargetsState.evaluateAndUpdateGuild(target.ID, guild.ID, result.percent, settings.NotificationThreshold)
		if !eval.sendIncrease && !eval.sendDecrease && !eval.sendRecover && !eval.sendComplete {
			continue
		}
		if eval.sendRecover {
			n.sendWatchTargetZeroRecoveryNotification(*settings.NotificationChannel, settings, target, result)
		}
		if eval.sendComplete {
			n.sendWatchTargetZeroCompletionNotification(*settings.NotificationChannel, settings, target, result)
		}
		if eval.sendIncrease {
			n.sendWatchTargetIncreaseNotification(*settings.NotificationChannel, settings, target, result, eval.tier)
		}
		if eval.sendDecrease {
			n.sendWatchTargetDecreaseNotification(*settings.NotificationChannel, settings, target, result, eval.tier)
		}
	}
	n.watchTargetsState.clearErrorNotified(target.ID)
}

func (n *Notifier) buildWatchTargetResult(target watchTargetConfig) (*watchTargetResult, error) {
	template, err := n.watchTargetsState.loadTemplate(target.Template)
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
		return nil, fmt.Errorf("watch origin out of range: %s", target.Origin)
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
	percent := 0.0
	if template.OpaqueCount > 0 {
		percent = float64(diffPixels) * 100 / float64(template.OpaqueCount)
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
	return &watchTargetResult{
		coord:      coord,
		template:   template,
		diffPixels: diffPixels,
		percent:    percent,
		wplaceURL:  utils.BuildWplaceURL(center.Lng, center.Lat, utils.ZoomFromImageSize(template.Width, template.Height)),
		fullsize:   fmt.Sprintf("%d-%d-%d-%d-%d-%d", coord.TileX, coord.TileY, coord.PixelX, coord.PixelY, template.Width, template.Height),
		livePNG:    livePNG,
		diffPNG:    diffPNG,
		mergedPNG:  mergedPNG,
	}, nil
}


func (n *Notifier) sendWatchTargetIncreaseNotification(
	channelID string,
	settings config.GuildSettings,
	target watchTargetConfig,
	result *watchTargetResult,
	tier Tier,
) {
	mentionStr := ""
	if result.percent >= settings.MentionThreshold && settings.MentionRole != nil {
		mentionStr = fmt.Sprintf("<@&%s> ", *settings.MentionRole)
	}
	tierDesc := "変動"
	switch tier {
	case Tier100:
		tierDesc = "100%に急増!!"
	case Tier90:
		tierDesc = "90%台に増加"
	case Tier80:
		tierDesc = "80%台に増加"
	case Tier70:
		tierDesc = "70%台に増加"
	case Tier60:
		tierDesc = "60%台に増加"
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
	}

	content := fmt.Sprintf("%s【Wplace速報】 🚨 差分率が%sしました！[現在%.2f%%]\n対象: `%s`", mentionStr, tierDesc, result.percent, target.Label)
	embed := n.buildWatchTargetEmbed("🏯 Wplace 荒らし検知 (追加監視)", target, result, getTierColor(tier))
	n.sendWatchTargetMessage(channelID, content, embed, target, result)
}

func (n *Notifier) sendWatchTargetDecreaseNotification(
	channelID string,
	settings config.GuildSettings,
	target watchTargetConfig,
	result *watchTargetResult,
	tier Tier,
) {
	content := fmt.Sprintf("【Wplace速報】 差分率が%sまで減少しました。[現在%.2f%%]\n対象: `%s`", tierRangeLabel(tier, settings.NotificationThreshold), result.percent, target.Label)
	embed := n.buildWatchTargetEmbed("🏯 Wplace 差分減少 (追加監視)", target, result, getTierColor(tier))
	n.sendWatchTargetMessage(channelID, content, embed, target, result)
}

func (n *Notifier) sendWatchTargetZeroRecoveryNotification(
	channelID string,
	_ config.GuildSettings,
	target watchTargetConfig,
	result *watchTargetResult,
) {
	content := fmt.Sprintf("🔔 【Wplace速報】変化検知 差分率: **%.2f%%**に上昇\n対象: `%s`", result.percent, target.Label)
	embed := n.buildWatchTargetEmbed("🟢 Wplace 変化検知 (追加監視)", target, result, 0x00FF00)
	n.sendWatchTargetMessage(channelID, content, embed, target, result)
}

func (n *Notifier) sendWatchTargetZeroCompletionNotification(
	channelID string,
	_ config.GuildSettings,
	target watchTargetConfig,
	result *watchTargetResult,
) {
	content := fmt.Sprintf("✅ 【Wplace速報】修復完了！ 差分率: **0.00%%** # Pixel Perfect!\n対象: `%s`", target.Label)
	embed := n.buildWatchTargetEmbed("🎉 Wplace 修復完了 (追加監視)", target, result, 0x00FF00)
	n.sendWatchTargetMessage(channelID, content, embed, target, result)
}

func (n *Notifier) buildWatchTargetEmbed(title string, target watchTargetConfig, result *watchTargetResult, colorCode int) *discordgo.MessageEmbed {
	embed := &discordgo.MessageEmbed{
		Title:       title,
		Description: fmt.Sprintf("対象 `%s` の監視結果", target.Label),
		Color:       colorCode,
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:   "ID",
				Value:  fmt.Sprintf("`%s`", target.ID),
				Inline: true,
			},
			{
				Name:   "差分率",
				Value:  fmt.Sprintf("%.2f%%", result.percent),
				Inline: true,
			},
			{
				Name:   "差分ピクセル",
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
				Value:  fmt.Sprintf("`!%s`", target.ID),
				Inline: true,
			},
			{
				Name:   "Wplace.live",
				Value:  fmt.Sprintf("[地図で見る](%s)\n`/get fullsize:%s`", result.wplaceURL, result.fullsize),
				Inline: false,
			},
		},
		Image: &discordgo.MessageEmbedImage{
			URL: "attachment://watch_preview.png",
		},
		Timestamp: time.Now().Format(time.RFC3339),
	}
	return embed
}

func (n *Notifier) sendWatchTargetManual(channelID string, target watchTargetConfig, result *watchTargetResult) {
	content := fmt.Sprintf("📌 追加監視 手動取得: `%s`", target.Label)
	embed := n.buildWatchTargetEmbed("📌 追加監視 手動取得", target, result, 0x3498DB)
	n.sendWatchTargetMessage(channelID, content, embed, target, result)
}

func (n *Notifier) sendWatchTargetMessage(
	channelID string,
	content string,
	embed *discordgo.MessageEmbed,
	target watchTargetConfig,
	result *watchTargetResult,
) {
	_, err := n.session.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
		Content: content,
		Embeds:  []*discordgo.MessageEmbed{embed},
		Files: []*discordgo.File{
			{
				Name:        "watch_preview.png",
				ContentType: "image/png",
				Reader:      bytes.NewReader(result.mergedPNG),
			},
		},
	})
	if err != nil {
		log.Printf("watch_targets: notify failed channel=%s target=%s err=%v", channelID, target.ID, err)
	}
}

func (n *Notifier) handleWatchTargetError(target watchTargetConfig, err error, notifyOnce bool) {
	log.Printf("watch_targets: target=%s error=%v", target.ID, err)
	if !notifyOnce {
		return
	}
	if !n.watchTargetsState.shouldNotifyError(target.ID) {
		return
	}
	n.sendWatchTargetErrorNotification(target, err)
}

func (n *Notifier) sendWatchTargetErrorNotification(target watchTargetConfig, err error) {
	for _, guild := range n.session.State.Guilds {
		settings := n.settings.GetGuildSettings(guild.ID)
		if !settings.AutoNotifyEnabled || settings.NotificationChannel == nil {
			continue
		}
		embed := &discordgo.MessageEmbed{
			Title:       "⚠️ 追加監視エラー",
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
		if _, sendErr := n.session.ChannelMessageSendEmbed(*settings.NotificationChannel, embed); sendErr != nil {
			log.Printf("watch_targets: error notify failed guild=%s target=%s err=%v", guild.ID, target.ID, sendErr)
		}
	}
}

func (w *watchTargetsRuntime) loadConfigs() ([]watchTargetConfig, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.configs != nil && time.Since(w.configsLoaded) < watchTargetsReloadTTL {
		return append([]watchTargetConfig(nil), w.configs...), nil
	}

	path := filepath.Join(w.dataDir, watchTargetsFileName)
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			w.configs = nil
			w.configsLoaded = time.Now()
			return nil, nil
		}
		return nil, err
	}
	cfgs, err := parseWatchTargetsConfig(raw)
	if err != nil {
		return nil, err
	}
	w.configs = cfgs
	w.configsLoaded = time.Now()
	return append([]watchTargetConfig(nil), w.configs...), nil
}

func (w *watchTargetsRuntime) findTargetByID(targetID string) (watchTargetConfig, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.configs == nil {
		return watchTargetConfig{}, false
	}
	for _, cfg := range w.configs {
		if cfg.ID == targetID {
			return cfg, true
		}
	}
	return watchTargetConfig{}, false
}

func (w *watchTargetsRuntime) tryStart(targetID string, now time.Time, interval time.Duration) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	st, ok := w.statuses[targetID]
	if !ok {
		st = &watchTargetStatus{
			GuildStates: make(map[string]*NotificationState),
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

func (w *watchTargetsRuntime) finish(targetID string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if st, ok := w.statuses[targetID]; ok {
		st.Running = false
	}
}

func (w *watchTargetsRuntime) evaluateAndUpdateGuild(targetID, guildID string, diffValue, threshold float64) watchTargetEval {
	w.mu.Lock()
	defer w.mu.Unlock()

	st, ok := w.statuses[targetID]
	if !ok {
		st = &watchTargetStatus{
			GuildStates: make(map[string]*NotificationState),
		}
		w.statuses[targetID] = st
	}
	if st.GuildStates == nil {
		st.GuildStates = make(map[string]*NotificationState)
	}
	gs, ok := st.GuildStates[guildID]
	if !ok {
		gs = &NotificationState{
			LastTier:         TierNone,
			MentionTriggered: false,
			WasZeroDiff:      true,
		}
		st.GuildStates[guildID] = gs
	}

	currentTier := calculateTier(diffValue, threshold)
	isZero := isZeroDiff(diffValue)
	ev := watchTargetEval{
		sendRecover:  gs.WasZeroDiff && !isZero,
		sendComplete: !gs.WasZeroDiff && isZero,
		tier:         currentTier,
	}
	if currentTier != gs.LastTier {
		if currentTier > gs.LastTier {
			ev.sendIncrease = true
		} else {
			ev.sendDecrease = true
		}
	}

	gs.LastTier = currentTier
	gs.WasZeroDiff = isZero
	return ev
}

func (w *watchTargetsRuntime) shouldNotifyError(targetID string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.errorNotified[targetID] {
		return false
	}
	w.errorNotified[targetID] = true
	return true
}

func (w *watchTargetsRuntime) clearErrorNotified(targetID string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.errorNotified, targetID)
}

func (w *watchTargetsRuntime) loadTemplate(templateRef string) (*watchTemplate, error) {
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
	opaque := 0
	for y := 0; y < nrgba.Bounds().Dy(); y++ {
		for x := 0; x < nrgba.Bounds().Dx(); x++ {
			if nrgba.Pix[y*nrgba.Stride+x*4+3] != 0 {
				opaque++
			}
		}
	}
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

func parseWatchTargetsConfig(raw []byte) ([]watchTargetConfig, error) {
	type rawTarget struct {
		ID              string `json:"id"`
		Label           string `json:"label"`
		Origin          string `json:"origin"`
		Template        string `json:"template"`
		TemplatePath    string `json:"template_path"`
		IntervalSeconds int    `json:"interval_seconds"`
		Interval        int    `json:"interval"`
	}

	build := func(id string, item rawTarget) (watchTargetConfig, error) {
		cfg := watchTargetConfig{
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
			return watchTargetConfig{}, fmt.Errorf("watch target id is empty")
		}
		if cfg.Label == "" {
			cfg.Label = cfg.ID
		}
		if cfg.Origin == "" || cfg.Template == "" {
			return watchTargetConfig{}, fmt.Errorf("watch target %s missing origin/template", cfg.ID)
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
		out := make([]watchTargetConfig, 0, len(root.Targets))
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
		out := make([]watchTargetConfig, 0, len(asMap))
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
		out := make([]watchTargetConfig, 0, len(asList))
		for i, item := range asList {
			cfg, err := build(strconv.Itoa(i), item)
			if err != nil {
				return nil, err
			}
			out = append(out, cfg)
		}
		return out, nil
	}
	return nil, fmt.Errorf("watch_targets.json format is invalid")
}

func parseWatchOrigin(value string) (*utils.Coordinate, error) {
	parts := strings.Split(strings.TrimSpace(value), "-")
	if len(parts) != 4 {
		return nil, fmt.Errorf("invalid origin format: %s", value)
	}
	vals := make([]int, 4)
	for i, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil {
			return nil, fmt.Errorf("invalid origin value: %s", value)
		}
		vals[i] = n
	}
	return &utils.Coordinate{
		TileX:  vals[0],
		TileY:  vals[1],
		PixelX: vals[2],
		PixelY: vals[3],
	}, nil
}

func watchAreaCenter(origin *utils.Coordinate, width, height int) *utils.LngLat {
	centerAbsX := float64(origin.TileX*utils.WplaceTileSize+origin.PixelX) + float64(width)/2
	centerAbsY := float64(origin.TileY*utils.WplaceTileSize+origin.PixelY) + float64(height)/2
	centerTileX := int(centerAbsX) / utils.WplaceTileSize
	centerTileY := int(centerAbsY) / utils.WplaceTileSize
	centerPixelX := int(centerAbsX) % utils.WplaceTileSize
	centerPixelY := int(centerAbsY) % utils.WplaceTileSize
	return utils.TilePixelCenterToLngLat(centerTileX, centerTileY, centerPixelX, centerPixelY)
}

func resolveTemplatePath(dataDir, ref string) (string, error) {
	cleanRef := filepath.Clean(strings.TrimSpace(ref))
	if cleanRef == "." || cleanRef == "" {
		return "", fmt.Errorf("template path is empty")
	}
	base := filepath.Clean(filepath.Join(dataDir, templateImageDirName))
	full := filepath.Clean(filepath.Join(base, cleanRef))
	basePrefix := base + string(filepath.Separator)
	if full != base && !strings.HasPrefix(full, basePrefix) {
		return "", fmt.Errorf("template path is outside template_img: %s", ref)
	}
	return full, nil
}
