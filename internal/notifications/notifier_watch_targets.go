package notifications

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

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
	LastHasDiff *bool
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

func (n *Notifier) runWatchTarget(target watchTargetConfig) {
	template, err := n.watchTargetsState.loadTemplate(target.Template)
	if err != nil {
		n.handleWatchTargetError(target, err, true)
		return
	}
	coord, err := parseWatchOrigin(target.Origin)
	if err != nil {
		n.handleWatchTargetError(target, err, true)
		return
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
		n.handleWatchTargetError(target, fmt.Errorf("watch origin out of range: %s", target.Origin), true)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	tilesData, err := wplace.DownloadTilesGrid(ctx, nil, startTileX, startTileY, tilesX, tilesY, 16)
	cancel()
	if err != nil {
		log.Printf("watch_targets: download failed for %s: %v", target.ID, err)
		return
	}

	cropRect := image.Rect(startPixelX, startPixelY, startPixelX+template.Width, startPixelY+template.Height)
	liveImg, err := wplace.CombineTilesCroppedImage(tilesData, utils.WplaceTileSize, utils.WplaceTileSize, tilesX, tilesY, cropRect)
	if err != nil {
		log.Printf("watch_targets: combine failed for %s: %v", target.ID, err)
		return
	}

	diffPixels := compareTemplateDiff(template, liveImg)
	hasDiff := diffPixels > 0
	changed, lastKnown := n.watchTargetsState.updateDiffStatus(target.ID, hasDiff)
	if !changed {
		return
	}

	percent := 0.0
	if template.OpaqueCount > 0 {
		percent = float64(diffPixels) * 100 / float64(template.OpaqueCount)
	}
	center := watchAreaCenter(coord, template.Width, template.Height)
	wplaceURL := utils.BuildWplaceURL(center.Lng, center.Lat, utils.ZoomFromImageSize(template.Width, template.Height))
	n.sendWatchTargetDiffNotification(target, hasDiff, lastKnown, diffPixels, template.OpaqueCount, percent, wplaceURL)
	n.watchTargetsState.clearErrorNotified(target.ID)
}

func compareTemplateDiff(template *watchTemplate, live *image.NRGBA) int {
	if template == nil || template.Img == nil || live == nil {
		return 0
	}
	bounds := template.Img.Bounds()
	if live.Bounds().Dx() != bounds.Dx() || live.Bounds().Dy() != bounds.Dy() {
		return template.OpaqueCount
	}
	diff := 0
	for y := 0; y < bounds.Dy(); y++ {
		for x := 0; x < bounds.Dx(); x++ {
			ti := y*template.Img.Stride + x*4
			alpha := template.Img.Pix[ti+3]
			if alpha == 0 {
				continue
			}
			li := y*live.Stride + x*4
			if template.Img.Pix[ti] != live.Pix[li] ||
				template.Img.Pix[ti+1] != live.Pix[li+1] ||
				template.Img.Pix[ti+2] != live.Pix[li+2] {
				diff++
			}
		}
	}
	return diff
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

func (n *Notifier) sendWatchTargetDiffNotification(
	target watchTargetConfig,
	hasDiff bool,
	lastKnown bool,
	diffPixels int,
	totalPixels int,
	percent float64,
	wplaceURL string,
) {
	for _, guild := range n.session.State.Guilds {
		settings := n.settings.GetGuildSettings(guild.ID)
		if !settings.AutoNotifyEnabled || settings.NotificationChannel == nil {
			continue
		}
		title := "✅ 追加監視: 修復完了"
		desc := fmt.Sprintf("対象 `%s` がテンプレート一致に戻りました。", target.Label)
		color := 0x2ECC71
		if hasDiff {
			title = "🚨 追加監視: 差分検知"
			desc = fmt.Sprintf("対象 `%s` に差分があります。", target.Label)
			color = 0xE74C3C
			if !lastKnown {
				desc = fmt.Sprintf("対象 `%s` に新たな差分が発生しました。", target.Label)
			}
		}

		embed := &discordgo.MessageEmbed{
			Title:       title,
			Description: desc,
			Color:       color,
			Fields: []*discordgo.MessageEmbedField{
				{
					Name:   "差分ピクセル",
					Value:  fmt.Sprintf("%d / %d (%.2f%%)", diffPixels, totalPixels, percent),
					Inline: true,
				},
				{
					Name:   "対象",
					Value:  fmt.Sprintf("`%s`", target.Label),
					Inline: true,
				},
				{
					Name:   "座標",
					Value:  fmt.Sprintf("`%s`", target.Origin),
					Inline: false,
				},
				{
					Name:   "Wplace.live",
					Value:  fmt.Sprintf("[地図で見る](%s)", wplaceURL),
					Inline: false,
				},
			},
			Timestamp: time.Now().Format(time.RFC3339),
		}

		if _, err := n.session.ChannelMessageSendEmbed(*settings.NotificationChannel, embed); err != nil {
			log.Printf("watch_targets: notify failed guild=%s target=%s err=%v", guild.ID, target.ID, err)
		}
	}
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

func (w *watchTargetsRuntime) tryStart(targetID string, now time.Time, interval time.Duration) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	st, ok := w.statuses[targetID]
	if !ok {
		st = &watchTargetStatus{}
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

func (w *watchTargetsRuntime) updateDiffStatus(targetID string, hasDiff bool) (changed bool, lastKnown bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	st, ok := w.statuses[targetID]
	if !ok {
		st = &watchTargetStatus{}
		w.statuses[targetID] = st
	}
	if st.LastHasDiff == nil {
		st.LastHasDiff = &hasDiff
		return hasDiff, false
	}
	prev := *st.LastHasDiff
	if prev == hasDiff {
		return false, prev
	}
	*st.LastHasDiff = hasDiff
	return true, true
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

func toNRGBAImage(src image.Image) *image.NRGBA {
	b := src.Bounds()
	dst := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			dst.Set(x, y, src.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return dst
}
