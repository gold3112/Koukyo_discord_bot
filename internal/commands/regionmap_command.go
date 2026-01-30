package commands

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"

	"github.com/bwmarrin/discordgo"
)

const (
	regionMapZoom            = 11
	regionMapSimplifiedZoom  = 7
	regionMapTileSize        = 256
	regionMapUserAgent       = "WplaceDiscordBot/2.3.1"
	regionMapMaxTiles        = 32
	regionMapMaxBytes        = 8 * 1024 * 1024
	regionMapPageSize        = 25
	regionMapTitleHeight     = 80
	regionMapTitleHeightSM   = 60
	regionMapFontSize        = 48
	regionMapFontSizeSM      = 18
	regionMapTitleFontSize   = 20
	regionMapTitleFontSizeSM = 16
	regionMapMaxConcurrent   = 16
	regionMapDBCacheTTL      = 2 * time.Minute
	regionMapBaseCacheMax    = 8

	regionMapSelectPrefix = "regionmap_select:"
	regionMapPagePrefix   = "regionmap_page:"
)

type RegionMapCommand struct{}

func NewRegionMapCommand() *RegionMapCommand {
	return &RegionMapCommand{}
}

type regionDBCache struct {
	mu        sync.Mutex
	data      RegionDB
	fetchedAt time.Time
}

var regionMapDBCache regionDBCache

type baseMapKey struct {
	city string
	zoom int
	minX int
	maxX int
	minY int
	maxY int
}

type baseMapCache struct {
	mu    sync.Mutex
	items map[baseMapKey]*image.RGBA
}

var regionMapBaseCache = baseMapCache{
	items: make(map[baseMapKey]*image.RGBA),
}

func (c *RegionMapCommand) Name() string { return "regionmap" }

func (c *RegionMapCommand) Description() string {
	return "指定地域のRegion配置マップを表示します"
}

func (c *RegionMapCommand) ExecuteText(s *discordgo.Session, m *discordgo.MessageCreate, args []string) error {
	_, err := s.ChannelMessageSend(m.ChannelID, "このコマンドはスラッシュコマンドで利用してください。")
	return err
}

func (c *RegionMapCommand) ExecuteSlash(s *discordgo.Session, i *discordgo.InteractionCreate) error {
	query := ""
	for _, opt := range i.ApplicationCommandData().Options {
		if opt.Name == "name" {
			query = strings.TrimSpace(opt.StringValue())
		}
	}
	if query == "" {
		return respondGet(s, i, "❌ 地域名を指定してください。例: /regionmap name:Uruma")
	}
	if err := respondDeferred(s, i); err != nil {
		return err
	}

	embed, file, components, err := buildRegionMapMessage(query, 0, "")
	if err != nil {
		return followupMessage(s, i, "❌ "+err.Error())
	}
	return sendRegionMapFollowup(s, i, embed, file, components)
}

func (c *RegionMapCommand) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "name",
				Description: "地域名 (例: Uruma)",
				Required:    true,
			},
		},
	}
}

func HandleRegionMapPagination(s *discordgo.Session, i *discordgo.InteractionCreate) {
	query, page, ok := parseRegionMapCustomID(i.MessageComponentData().CustomID, regionMapPagePrefix)
	if !ok {
		return
	}
	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredMessageUpdate,
	})
	go func() {
		embed, file, components, err := buildRegionMapMessage(query, page, "")
		if err != nil {
			msg := "❌ エラー: " + err.Error()
			_, _ = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
				Content: &msg,
			})
			return
		}
		embeds := []*discordgo.MessageEmbed{embed}
		comps := components
		_, _ = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Embeds:     &embeds,
			Components: &comps,
			Files:      buildOptionalFiles(file),
		})
	}()
}

func HandleRegionMapSelect(s *discordgo.Session, i *discordgo.InteractionCreate) {
	query, page, ok := parseRegionMapCustomID(i.MessageComponentData().CustomID, regionMapSelectPrefix)
	if !ok {
		return
	}
	values := i.MessageComponentData().Values
	if len(values) == 0 {
		return
	}
	highlight := values[0]
	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredMessageUpdate,
	})
	go func() {
		embed, file, components, err := buildRegionMapMessage(query, page, highlight)
		if err != nil {
			msg := "❌ エラー: " + err.Error()
			_, _ = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
				Content: &msg,
			})
			return
		}
		embeds := []*discordgo.MessageEmbed{embed}
		comps := components
		_, _ = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Embeds:     &embeds,
			Components: &comps,
			Files:      buildOptionalFiles(file),
		})
	}()
}

func buildRegionMapMessage(query string, page int, highlight string) (*discordgo.MessageEmbed, *discordgo.File, []discordgo.MessageComponent, error) {
	db, err := getRegionDBCached()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("Regionデータベースの読み込みに失敗しました")
	}
	regions := searchRegionsByCity(db, query)
	if len(regions) == 0 {
		return nil, nil, nil, fmt.Errorf("該当Regionが見つかりません。例: Uruma")
	}

	names := sortedRegionNames(regions)
	page = clampPage(page, len(names), regionMapPageSize)

	data, contentType, filename, err := generateRegionMapImage(query, regions, highlight)
	if err != nil {
		return nil, nil, nil, err
	}
	file := &discordgo.File{
		Name:        filename,
		ContentType: contentType,
		Reader:      bytes.NewReader(data),
	}

	description := fmt.Sprintf("地域: %s / 件数: %d", query, len(regions))
	if highlight != "" {
		description += fmt.Sprintf("\n選択中: %s", highlight)
	}
	embed := &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("🗾 %s Region Map", query),
		Description: description,
		Color:       0x3498DB,
		Image: &discordgo.MessageEmbedImage{
			URL: "attachment://" + filename,
		},
		Footer: &discordgo.MessageEmbedFooter{
			Text: fmt.Sprintf("ページ %d / %d | Map data © OpenStreetMap contributors (openstreetmap.fr/hot)", page+1, totalPages(len(names), regionMapPageSize)),
		},
		Timestamp: time.Now().Format(time.RFC3339),
	}

	components := buildRegionMapComponents(query, names, page)
	return embed, file, components, nil
}

func getRegionDBCached() (RegionDB, error) {
	regionMapDBCache.mu.Lock()
	defer regionMapDBCache.mu.Unlock()
	if regionMapDBCache.data != nil && time.Since(regionMapDBCache.fetchedAt) < regionMapDBCacheTTL {
		return regionMapDBCache.data, nil
	}
	db, err := loadRegionDB(regionDBURL)
	if err != nil {
		return nil, err
	}
	regionMapDBCache.data = db
	regionMapDBCache.fetchedAt = time.Now()
	return db, nil
}

func buildRegionMapComponents(query string, names []string, page int) []discordgo.MessageComponent {
	total := totalPages(len(names), regionMapPageSize)
	start := page * regionMapPageSize
	end := start + regionMapPageSize
	if end > len(names) {
		end = len(names)
	}
	options := make([]discordgo.SelectMenuOption, 0, end-start)
	for i := start; i < end; i++ {
		name := names[i]
		options = append(options, discordgo.SelectMenuOption{
			Label: truncateRegionLabel(name, 100),
			Value: name,
		})
	}
	encoded := encodeRegionMapKey(query)
	components := []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.SelectMenu{
					CustomID:    fmt.Sprintf("%s%s:%d", regionMapSelectPrefix, encoded, page),
					Placeholder: "Regionを選択",
					Options:     options,
				},
			},
		},
	}
	if total > 1 {
		components = append(components, discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{
					Label:    "前へ",
					Style:    discordgo.PrimaryButton,
					CustomID: fmt.Sprintf("%s%s:%d", regionMapPagePrefix, encoded, page-1),
					Disabled: page <= 0,
				},
				discordgo.Button{
					Label:    "次へ",
					Style:    discordgo.PrimaryButton,
					CustomID: fmt.Sprintf("%s%s:%d", regionMapPagePrefix, encoded, page+1),
					Disabled: page >= total-1,
				},
			},
		})
	}
	return components
}

func sendRegionMapFollowup(s *discordgo.Session, i *discordgo.InteractionCreate, embed *discordgo.MessageEmbed, file *discordgo.File, components []discordgo.MessageComponent) error {
	params := &discordgo.WebhookParams{
		Embeds:     []*discordgo.MessageEmbed{embed},
		Files:      buildOptionalFiles(file),
		Components: components,
	}
	_, err := s.FollowupMessageCreate(i.Interaction, false, params)
	return err
}

func parseRegionMapCustomID(customID, prefix string) (string, int, bool) {
	if !strings.HasPrefix(customID, prefix) {
		return "", 0, false
	}
	payload := strings.TrimPrefix(customID, prefix)
	parts := strings.SplitN(payload, ":", 2)
	if len(parts) != 2 {
		return "", 0, false
	}
	query, err := decodeRegionMapKey(parts[0])
	if err != nil {
		return "", 0, false
	}
	page, err := strconv.Atoi(parts[1])
	if err != nil {
		return "", 0, false
	}
	return query, page, true
}

func encodeRegionMapKey(value string) string {
	escaped := url.QueryEscape(strings.TrimSpace(value))
	return base64.RawURLEncoding.EncodeToString([]byte(escaped))
}

func decodeRegionMapKey(value string) (string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return "", err
	}
	return url.QueryUnescape(string(raw))
}

func truncateRegionLabel(value string, max int) string {
	if len(value) <= max {
		return value
	}
	if max <= 3 {
		return value[:max]
	}
	return value[:max-3] + "..."
}

func totalPages(total, pageSize int) int {
	if total <= 0 {
		return 1
	}
	return (total + pageSize - 1) / pageSize
}

func clampPage(page, total, pageSize int) int {
	maxPage := 0
	if total > 0 {
		maxPage = (total - 1) / pageSize
	}
	if page < 0 {
		return 0
	}
	if page > maxPage {
		return maxPage
	}
	return page
}

func searchRegionsByCity(database RegionDB, cityName string) map[string]Region {
	results := make(map[string]Region)
	query := strings.ToLower(strings.TrimSpace(cityName))
	if query == "" {
		return results
	}
	for regionName, info := range database {
		lower := strings.ToLower(regionName)
		if strings.HasPrefix(lower, query+"#") {
			results[regionName] = info
		}
	}
	return results
}

func sortedRegionNames(regions map[string]Region) []string {
	names := make([]string, 0, len(regions))
	for name := range regions {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		ni, okI := parseRegionNumber(names[i])
		nj, okJ := parseRegionNumber(names[j])
		if okI && okJ && ni != nj {
			return ni < nj
		}
		return names[i] < names[j]
	})
	return names
}

func parseRegionNumber(name string) (int, bool) {
	parts := strings.SplitN(name, "#", 2)
	if len(parts) != 2 {
		return 0, false
	}
	n, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, false
	}
	return n, true
}

func generateRegionMapImage(cityName string, regions map[string]Region, highlight string) ([]byte, string, string, error) {
	minRX, maxRX, minRY, maxRY, ok := calculateRegionBounds(regions)
	if !ok {
		return nil, "", "", fmt.Errorf("Region座標が不正です")
	}

	minTX := minRX * 4
	maxTX := (maxRX+1)*4 - 1
	minTY := minRY * 4
	maxTY := (maxRY+1)*4 - 1

	tileWidth := maxTX - minTX + 1
	tileHeight := maxTY - minTY + 1
	if tileWidth > regionMapMaxTiles || tileHeight > regionMapMaxTiles {
		return generateSimplifiedRegionMap(cityName, regions, highlight)
	}

	data, err := generateFullRegionMap(cityName, regions, highlight, minTX, maxTX, minTY, maxTY)
	if err != nil {
		return generateSimplifiedRegionMap(cityName, regions, highlight)
	}
	if len(data) > regionMapMaxBytes {
		return generateSimplifiedRegionMap(cityName, regions, highlight)
	}
	return data, "image/png", "region_map.png", nil
}

func generateFullRegionMap(cityName string, regions map[string]Region, highlight string, minTX, maxTX, minTY, maxTY int) ([]byte, error) {
	tileWidth := maxTX - minTX + 1
	tileHeight := maxTY - minTY + 1
	mapWidth := tileWidth * regionMapTileSize
	mapHeight := tileHeight * regionMapTileSize
	key := baseMapKey{
		city: strings.ToLower(cityName),
		zoom: regionMapZoom,
		minX: minTX,
		maxX: maxTX,
		minY: minTY,
		maxY: maxTY,
	}
	baseMap := getBaseMapCached(key)
	if baseMap == nil {
		baseMap = buildBaseMap(mapWidth, mapHeight, minTX, maxTX, minTY, maxTY, regionMapZoom)
		storeBaseMapCached(key, baseMap)
	}
	baseMap = cloneRGBA(baseMap)

	for regionName, info := range regions {
		rx, ry := info.RegionCoords[0], info.RegionCoords[1]
		x1 := (rx*4 - minTX) * regionMapTileSize
		y1 := (ry*4 - minTY) * regionMapTileSize
		x2 := x1 + 4*regionMapTileSize
		y2 := y1 + 4*regionMapTileSize

		isHighlighted := regionName == highlight
		var overlay, border color.RGBA
		borderWidth := 4
		if isHighlighted {
			overlay = color.RGBA{255, 215, 0, 100}
			border = color.RGBA{255, 165, 0, 255}
			borderWidth = 8
		} else {
			overlay = color.RGBA{100, 149, 237, 60}
			border = color.RGBA{70, 130, 220, 200}
		}
		fillRect(baseMap, x1, y1, x2, y2, overlay)
		strokeRect(baseMap, x1, y1, x2, y2, border, borderWidth)

		numText := regionLabel(regionName, info)
		drawCenteredText(baseMap, numText, x1, y1, x2, y2, isHighlighted, regionMapFontSize)
	}

	titleHeight := regionMapTitleHeight
	finalImage := image.NewRGBA(image.Rect(0, 0, mapWidth, mapHeight+titleHeight))
	draw.Draw(finalImage, finalImage.Bounds(), &image.Uniform{C: color.RGBA{0x2C, 0x3E, 0x50, 0xFF}}, image.Point{}, draw.Src)
	draw.Draw(finalImage, image.Rect(0, titleHeight, mapWidth, mapHeight+titleHeight), baseMap, image.Point{}, draw.Src)
	title := fmt.Sprintf("🗾 %s Region Map (%d regions)", cityName, len(regions))
	drawTitle(finalImage, title, mapWidth, titleHeight, regionMapTitleFontSize)

	var buf bytes.Buffer
	if err := png.Encode(&buf, finalImage); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func generateSimplifiedRegionMap(cityName string, regions map[string]Region, highlight string) ([]byte, string, string, error) {
	minRX, maxRX, minRY, maxRY, ok := calculateRegionBounds(regions)
	if !ok {
		return nil, "", "", fmt.Errorf("Region座標が不正です")
	}

	scale := 4
	minTX := minRX / scale
	maxTX := maxRX / scale
	minTY := minRY / scale
	maxTY := maxRY / scale

	tilesX := maxTX - minTX + 1
	tilesY := maxTY - minTY + 1

	mapWidth := tilesX * regionMapTileSize
	mapHeight := tilesY * regionMapTileSize
	key := baseMapKey{
		city: strings.ToLower(cityName),
		zoom: regionMapSimplifiedZoom,
		minX: minTX,
		maxX: maxTX,
		minY: minTY,
		maxY: maxTY,
	}
	baseMap := getBaseMapCached(key)
	if baseMap == nil {
		baseMap = buildBaseMap(mapWidth, mapHeight, minTX, maxTX, minTY, maxTY, regionMapSimplifiedZoom)
		storeBaseMapCached(key, baseMap)
	}
	baseMap = cloneRGBA(baseMap)

	cellSize := regionMapTileSize / scale
	for regionName, info := range regions {
		rx, ry := info.RegionCoords[0], info.RegionCoords[1]
		col := rx - minRX
		row := ry - minRY
		x1 := col * cellSize
		y1 := row * cellSize
		x2 := x1 + cellSize
		y2 := y1 + cellSize

		isHighlighted := regionName == highlight
		var overlay, border color.RGBA
		borderWidth := 2
		if isHighlighted {
			overlay = color.RGBA{255, 215, 0, 100}
			border = color.RGBA{255, 140, 0, 200}
		} else {
			overlay = color.RGBA{100, 149, 237, 50}
			border = color.RGBA{70, 130, 220, 150}
		}
		fillRect(baseMap, x1, y1, x2, y2, overlay)
		strokeRect(baseMap, x1, y1, x2, y2, border, borderWidth)

		numText := regionLabel(regionName, info)
		drawCenteredText(baseMap, numText, x1, y1, x2, y2, isHighlighted, regionMapFontSizeSM)
	}

	titleHeight := regionMapTitleHeightSM
	finalImage := image.NewRGBA(image.Rect(0, 0, mapWidth, mapHeight+titleHeight))
	draw.Draw(finalImage, finalImage.Bounds(), &image.Uniform{C: color.RGBA{0x2C, 0x3E, 0x50, 0xFF}}, image.Point{}, draw.Src)
	draw.Draw(finalImage, image.Rect(0, titleHeight, mapWidth, mapHeight+titleHeight), baseMap, image.Point{}, draw.Src)
	title := fmt.Sprintf("🗾 %s Region Map (%d regions) - Simplified", cityName, len(regions))
	drawTitle(finalImage, title, mapWidth, titleHeight, regionMapTitleFontSizeSM)

	data, contentType, filename, err := encodeRegionMap(finalImage)
	if err != nil {
		return nil, "", "", err
	}
	return data, contentType, filename, nil
}

func calculateRegionBounds(regions map[string]Region) (int, int, int, int, bool) {
	if len(regions) == 0 {
		return 0, 0, 0, 0, false
	}
	first := true
	var minRX, maxRX, minRY, maxRY int
	for _, info := range regions {
		rx := info.RegionCoords[0]
		ry := info.RegionCoords[1]
		if first {
			minRX, maxRX, minRY, maxRY = rx, rx, ry, ry
			first = false
			continue
		}
		if rx < minRX {
			minRX = rx
		}
		if rx > maxRX {
			maxRX = rx
		}
		if ry < minRY {
			minRY = ry
		}
		if ry > maxRY {
			maxRY = ry
		}
	}
	return minRX, maxRX, minRY, maxRY, true
}

func regionLabel(name string, info Region) string {
	if n, ok := parseRegionNumber(name); ok {
		return fmt.Sprintf("#%d", n)
	}
	if n, ok := parseRegionNumber(info.Name); ok {
		return fmt.Sprintf("#%d", n)
	}
	return "#?"
}

func fillRect(img *image.RGBA, x1, y1, x2, y2 int, c color.RGBA) {
	rect := image.Rect(x1, y1, x2, y2)
	draw.Draw(img, rect, &image.Uniform{C: c}, image.Point{}, draw.Over)
}

func strokeRect(img *image.RGBA, x1, y1, x2, y2 int, c color.RGBA, width int) {
	if width <= 0 {
		width = 1
	}
	for i := 0; i < width; i++ {
		top := image.Rect(x1+i, y1+i, x2-i, y1+i+1)
		bottom := image.Rect(x1+i, y2-i-1, x2-i, y2-i)
		left := image.Rect(x1+i, y1+i, x1+i+1, y2-i)
		right := image.Rect(x2-i-1, y1+i, x2-i, y2-i)
		draw.Draw(img, top, &image.Uniform{C: c}, image.Point{}, draw.Over)
		draw.Draw(img, bottom, &image.Uniform{C: c}, image.Point{}, draw.Over)
		draw.Draw(img, left, &image.Uniform{C: c}, image.Point{}, draw.Over)
		draw.Draw(img, right, &image.Uniform{C: c}, image.Point{}, draw.Over)
	}
}

func drawCenteredText(img *image.RGBA, text string, x1, y1, x2, y2 int, highlight bool, size float64) {
	face := resolveFontFace(size, basicfont.Face7x13)
	textWidth := font.MeasureString(face, text).Ceil()
	textHeight := face.Metrics().Height.Ceil()
	ascent := face.Metrics().Ascent.Ceil()

	textX := x1 + (x2-x1-textWidth)/2
	textY := y1 + (y2-y1-textHeight)/2 + ascent

	outline := color.RGBA{255, 255, 255, 255}
	for _, dx := range []int{-2, 0, 2} {
		for _, dy := range []int{-2, 0, 2} {
			if dx == 0 && dy == 0 {
				continue
			}
			drawText(img, text, textX+dx, textY+dy, outline, face)
		}
	}
	textColor := color.RGBA{0, 0, 139, 255}
	if highlight {
		textColor = color.RGBA{255, 140, 0, 255}
	}
	drawText(img, text, textX, textY, textColor, face)
}

func drawTitle(img *image.RGBA, text string, width, height int, size float64) {
	face := resolveFontFace(size, basicfont.Face7x13)
	textWidth := font.MeasureString(face, text).Ceil()
	ascent := face.Metrics().Ascent.Ceil()
	x := (width - textWidth) / 2
	y := (height-ascent)/2 + ascent
	drawText(img, text, x, y, color.RGBA{255, 255, 255, 255}, face)
}

func drawText(img *image.RGBA, text string, x, y int, c color.RGBA, face font.Face) {
	d := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(c),
		Face: face,
		Dot:  fixed.P(x, y),
	}
	d.DrawString(text)
}

func resolveFontFace(size float64, fallback font.Face) font.Face {
	face := getGoFontFace(size)
	if face != nil {
		return face
	}
	return fallback
}

var (
	gofontOnce      sync.Once
	gofontFaceCache = make(map[float64]font.Face)
	gofontErr       error
	gofontMu        sync.Mutex
	gofontData      *opentype.Font
)

func getGoFontFace(size float64) font.Face {
	gofontOnce.Do(func() {
		gofontData, gofontErr = opentype.Parse(goregular.TTF)
	})
	if gofontErr != nil || gofontData == nil {
		return nil
	}
	gofontMu.Lock()
	defer gofontMu.Unlock()
	if face, ok := gofontFaceCache[size]; ok {
		return face
	}
	face, err := opentype.NewFace(gofontData, &opentype.FaceOptions{
		Size:    size,
		DPI:     72,
		Hinting: font.HintingFull,
	})
	if err != nil {
		return nil
	}
	gofontFaceCache[size] = face
	return face
}

func encodeRegionMap(img image.Image) ([]byte, string, string, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err == nil {
		if buf.Len() <= regionMapMaxBytes {
			return buf.Bytes(), "image/png", "region_map.png", nil
		}
	}
	buf.Reset()
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85}); err != nil {
		return nil, "", "", err
	}
	return buf.Bytes(), "image/jpeg", "region_map.jpg", nil
}

func buildBaseMap(mapWidth, mapHeight, minTX, maxTX, minTY, maxTY, zoom int) *image.RGBA {
	baseMap := image.NewRGBA(image.Rect(0, 0, mapWidth, mapHeight))
	draw.Draw(baseMap, baseMap.Bounds(), &image.Uniform{C: color.RGBA{0xE8, 0xE8, 0xE8, 0xFF}}, image.Point{}, draw.Src)
	tiles := regionMapGenerator.fetchTiles(minTX, maxTX, minTY, maxTY, zoom)
	for key, tile := range tiles {
		if tile == nil {
			continue
		}
		tileRGBA := toRGBA(tile)
		xOffset := (key.x - minTX) * regionMapTileSize
		yOffset := (key.y - minTY) * regionMapTileSize
		draw.Draw(baseMap, image.Rect(xOffset, yOffset, xOffset+regionMapTileSize, yOffset+regionMapTileSize), tileRGBA, image.Point{}, draw.Src)
	}
	return baseMap
}

func getBaseMapCached(key baseMapKey) *image.RGBA {
	regionMapBaseCache.mu.Lock()
	defer regionMapBaseCache.mu.Unlock()
	if img, ok := regionMapBaseCache.items[key]; ok {
		return img
	}
	return nil
}

func storeBaseMapCached(key baseMapKey, img *image.RGBA) {
	regionMapBaseCache.mu.Lock()
	defer regionMapBaseCache.mu.Unlock()
	if len(regionMapBaseCache.items) >= regionMapBaseCacheMax {
		regionMapBaseCache.items = make(map[baseMapKey]*image.RGBA)
	}
	regionMapBaseCache.items[key] = img
}

func toRGBA(src image.Image) *image.RGBA {
	b := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(dst, dst.Bounds(), src, b.Min, draw.Src)
	return dst
}

func cloneRGBA(src *image.RGBA) *image.RGBA {
	if src == nil {
		return nil
	}
	dst := image.NewRGBA(src.Bounds())
	copy(dst.Pix, src.Pix)
	return dst
}

type tileKey struct {
	x int
	y int
}

type RegionMapGenerator struct {
	client    *http.Client
	userAgent string
	cache     map[string]image.Image
	mu        sync.Mutex
	rr        int
}

var regionMapGenerator = newRegionMapGenerator()

func newRegionMapGenerator() *RegionMapGenerator {
	transport := &http.Transport{
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   32,
		MaxConnsPerHost:       32,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	return &RegionMapGenerator{
		client: &http.Client{
			Timeout:   12 * time.Second,
			Transport: transport,
		},
		userAgent: regionMapUserAgent,
		cache:     make(map[string]image.Image),
	}
}

func (g *RegionMapGenerator) fetchTiles(minTX, maxTX, minTY, maxTY, zoom int) map[tileKey]image.Image {
	result := make(map[tileKey]image.Image)
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, regionMapMaxConcurrent)
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()

	for ty := minTY; ty <= maxTY; ty++ {
		for tx := minTX; tx <= maxTX; tx++ {
			wg.Add(1)
			sem <- struct{}{}
			go func(x, y int) {
				defer wg.Done()
				defer func() { <-sem }()
				tile, _ := g.getTile(ctx, zoom, x, y)
				mu.Lock()
				result[tileKey{x: x, y: y}] = tile
				mu.Unlock()
			}(tx, ty)
		}
	}
	wg.Wait()
	return result
}

func (g *RegionMapGenerator) getTile(ctx context.Context, zoom, x, y int) (image.Image, error) {
	cacheKey := fmt.Sprintf("%d/%d/%d", zoom, x, y)
	g.mu.Lock()
	if img, ok := g.cache[cacheKey]; ok {
		g.mu.Unlock()
		return img, nil
	}
	g.mu.Unlock()

	subdomain := g.nextSubdomain()
	tileURL := fmt.Sprintf("https://%s.tile.openstreetmap.fr/hot/%d/%d/%d.png", subdomain, zoom, x, y)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, tileURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", g.userAgent)

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tile fetch failed: %s", resp.Status)
	}
	img, _, err := image.Decode(resp.Body)
	if err != nil {
		return nil, err
	}

	g.mu.Lock()
	g.cache[cacheKey] = img
	g.mu.Unlock()
	return img, nil
}

func (g *RegionMapGenerator) nextSubdomain() string {
	subdomains := []string{"a", "b", "c"}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.rr++
	return subdomains[g.rr%len(subdomains)]
}
