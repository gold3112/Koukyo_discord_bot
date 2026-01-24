package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"io"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"

	"Koukyo_discord_bot/internal/utils"

	"github.com/bwmarrin/discordgo"
)

type GetCommand struct {
	limiter *utils.RateLimiter
}

func NewGetCommand(limiter *utils.RateLimiter) *GetCommand {
	return &GetCommand{limiter: limiter}
}

func (c *GetCommand) Name() string {
	return "get"
}

func (c *GetCommand) Description() string {
	return "画像やデータを取得します。"
}

func (c *GetCommand) ExecuteText(s *discordgo.Session, m *discordgo.MessageCreate, args []string) error {
	_, err := s.ChannelMessageSend(m.ChannelID, "このコマンドはスラッシュコマンドで利用してください。")
	return err
}

func (c *GetCommand) ExecuteSlash(s *discordgo.Session, i *discordgo.InteractionCreate) error {
	options := i.ApplicationCommandData().Options
	var (
		coords   string
		region   string
		fullsize string
	)
	for _, opt := range options {
		switch opt.Name {
		case "coords":
			coords = opt.StringValue()
		case "region":
			region = opt.StringValue()
		case "fullsize":
			fullsize = opt.StringValue()
		}
	}

	if coords != "" {
		parts := strings.Split(coords, "-")
		if len(parts) != 2 {
			return respondGet(s, i, "❌ 座標形式が正しくありません: TlX-TlY 例: 1818-806")
		}
		tileX, errX := strconv.Atoi(parts[0])
		tileY, errY := strconv.Atoi(parts[1])
		if errX != nil || errY != nil {
			return respondGet(s, i, "❌ 座標値が不正です。整数で指定してください。")
		}
		if tileX < 0 || tileX >= 2048 || tileY < 0 || tileY >= 2048 {
			return respondGet(s, i, fmt.Sprintf("❌ タイル座標が範囲外です: %d-%d 有効範囲: 0～2047", tileX, tileY))
		}

		imageData, err := c.downloadTile(context.Background(), tileX, tileY)
		if err != nil {
			return respondGet(s, i, fmt.Sprintf("❌ タイル画像のダウンロードに失敗しました: %v", err))
		}
		latLng := utils.TilePixelToLngLat(tileX, tileY, utils.WplaceTileSize/2, utils.WplaceTileSize/2)
		wplaceURL := utils.BuildWplaceURL(latLng.Lng, latLng.Lat, calculateZoomFromWH(utils.WplaceTileSize, utils.WplaceTileSize))
		filename := fmt.Sprintf("tile_%d-%d.png", tileX, tileY)
		embed := &discordgo.MessageEmbed{
			Title:       fmt.Sprintf("🗺️ タイル画像: %d-%d", tileX, tileY),
			Description: fmt.Sprintf("[Wplaceで開く](%s)", wplaceURL),
			Color:       0x5865F2,
			Fields: []*discordgo.MessageEmbedField{
				{
					Name:   "タイル座標",
					Value:  fmt.Sprintf("`%d-%d`", tileX, tileY),
					Inline: true,
				},
				{
					Name:   "中心座標",
					Value:  fmt.Sprintf("`%.6f, %.6f`", latLng.Lng, latLng.Lat),
					Inline: true,
				},
			},
			Image: &discordgo.MessageEmbedImage{
				URL: "attachment://" + filename,
			},
		}
		return sendImageWithEmbed(s, i, imageData, filename, embed)
	}

	if region != "" {
		if err := respondDeferred(s, i); err != nil {
			return err
		}
		db, err := loadRegionDB("data/region_database.json")
		if err != nil {
			return followupMessage(s, i, "Regionデータベースの読み込みに失敗しました。")
		}
		reg, ok := findRegionByName(db, region)
		if !ok {
			return followupMessage(s, i, "❌ Regionが見つかりません。例: Tokyo#1, Osaka#1 など")
		}

		minTileX, minTileY := reg.TileRange.Min[0], reg.TileRange.Min[1]
		maxTileX, maxTileY := reg.TileRange.Max[0], reg.TileRange.Max[1]
		if minTileX < 0 || minTileY < 0 || maxTileX >= utils.WplaceTilesPerEdge || maxTileY >= utils.WplaceTilesPerEdge {
			return followupMessage(s, i, fmt.Sprintf("❌ Regionタイル範囲が無効です: X[%d-%d] Y[%d-%d]", minTileX, maxTileX, minTileY, maxTileY))
		}
		gridCols := maxTileX - minTileX + 1
		gridRows := maxTileY - minTileY + 1
		if gridCols <= 0 || gridRows <= 0 {
			return followupMessage(s, i, "❌ Regionタイル範囲が無効です。")
		}

		tilesData := make([][]byte, 0, gridCols*gridRows)
		for ty := minTileY; ty <= maxTileY; ty++ {
			for tx := minTileX; tx <= maxTileX; tx++ {
				data, err := c.downloadTile(context.Background(), tx, ty)
				if err != nil {
					return followupMessage(s, i, fmt.Sprintf("❌ タイル画像のダウンロードに失敗しました: %v", err))
				}
				tilesData = append(tilesData, data)
			}
		}

		buf, err := combineTiles(tilesData, utils.WplaceTileSize, utils.WplaceTileSize, gridCols, gridRows)
		if err != nil {
			return followupMessage(s, i, fmt.Sprintf("❌ 画像結合に失敗しました: %v", err))
		}
		displayName := fmt.Sprintf("%s_%d", reg.Name, reg.CountryID)
		filename := fmt.Sprintf("%s_full.png", strings.ReplaceAll(displayName, "#", "_"))
		centerLat := reg.CenterLatLng[0]
		centerLng := reg.CenterLatLng[1]
		imageWidth := gridCols * utils.WplaceTileSize
		imageHeight := gridRows * utils.WplaceTileSize
		wplaceURL := utils.BuildWplaceURL(centerLng, centerLat, calculateZoomFromWH(imageWidth, imageHeight))
		embed := &discordgo.MessageEmbed{
			Title: fmt.Sprintf("🗺️ %s 全域画像", displayName),
			Color: 0x5865F2,
			Fields: []*discordgo.MessageEmbedField{
				{
					Name:   "Region ID",
					Value:  fmt.Sprintf("`%d`", reg.RegionID),
					Inline: true,
				},
				{
					Name:   "City ID",
					Value:  fmt.Sprintf("`%d`", reg.CityID),
					Inline: true,
				},
				{
					Name:   "タイル範囲",
					Value:  fmt.Sprintf("X[%d-%d] Y[%d-%d]", minTileX, maxTileX, minTileY, maxTileY),
					Inline: false,
				},
				{
					Name:   "画像サイズ",
					Value:  fmt.Sprintf("%dx%dpx (%d×%dpx)", imageWidth, imageHeight, imageWidth, imageHeight),
					Inline: true,
				},
				{
					Name:   "タイル数",
					Value:  fmt.Sprintf("%dタイル (%d×%d)", gridCols*gridRows, gridCols, gridRows),
					Inline: true,
				},
				{
					Name:   "中心座標",
					Value:  fmt.Sprintf("緯度: %.4f, 経度: %.4f", centerLat, centerLng),
					Inline: false,
				},
				{
					Name:   "Wplace.live",
					Value:  fmt.Sprintf("[地図で見る](%s)", wplaceURL),
					Inline: false,
				},
			},
			Image: &discordgo.MessageEmbedImage{
				URL: "attachment://" + filename,
			},
		}
		return sendImageFollowup(s, i, buf.Bytes(), filename, embed)
	}

	if fullsize != "" {
		if err := respondDeferred(s, i); err != nil {
			return err
		}
		parts := strings.Split(fullsize, "-")
		var (
			tileX  int
			tileY  int
			pixelX int
			pixelY int
			width  int
			height int
		)
		switch len(parts) {
		case 6:
			var err error
			tileX, err = strconv.Atoi(parts[0])
			if err != nil {
				return followupMessage(s, i, fmt.Sprintf("❌ 座標値が不正です: %s", fullsize))
			}
			tileY, err = strconv.Atoi(parts[1])
			if err != nil {
				return followupMessage(s, i, fmt.Sprintf("❌ 座標値が不正です: %s", fullsize))
			}
			pixelX, err = strconv.Atoi(parts[2])
			if err != nil {
				return followupMessage(s, i, fmt.Sprintf("❌ 座標値が不正です: %s", fullsize))
			}
			pixelY, err = strconv.Atoi(parts[3])
			if err != nil {
				return followupMessage(s, i, fmt.Sprintf("❌ 座標値が不正です: %s", fullsize))
			}
			width, err = strconv.Atoi(parts[4])
			if err != nil {
				return followupMessage(s, i, fmt.Sprintf("❌ サイズが不正です: %s", fullsize))
			}
			height, err = strconv.Atoi(parts[5])
			if err != nil {
				return followupMessage(s, i, fmt.Sprintf("❌ サイズが不正です: %s", fullsize))
			}
		case 8:
			values := make([]int, 0, 8)
			for _, part := range parts {
				val, err := strconv.Atoi(part)
				if err != nil {
					return followupMessage(s, i, fmt.Sprintf("❌ 座標値が不正です: %s", fullsize))
				}
				values = append(values, val)
			}
			absX1 := values[0]*utils.WplaceTileSize + values[2]
			absY1 := values[1]*utils.WplaceTileSize + values[3]
			absX2 := values[4]*utils.WplaceTileSize + values[6]
			absY2 := values[5]*utils.WplaceTileSize + values[7]
			if absX1 > absX2 {
				absX1, absX2 = absX2, absX1
			}
			if absY1 > absY2 {
				absY1, absY2 = absY2, absY1
			}
			tileX = absX1 / utils.WplaceTileSize
			tileY = absY1 / utils.WplaceTileSize
			pixelX = absX1 % utils.WplaceTileSize
			pixelY = absY1 % utils.WplaceTileSize
			width = absX2 - absX1
			height = absY2 - absY1
		default:
			return followupMessage(s, i, fmt.Sprintf("❌ fullsize形式が正しくありません: %s", fullsize))
		}

		if tileX < 0 || tileX >= utils.WplaceTilesPerEdge || tileY < 0 || tileY >= utils.WplaceTilesPerEdge {
			return followupMessage(s, i, fmt.Sprintf("❌ タイル座標が範囲外です: %d-%d 有効範囲: 0～2047", tileX, tileY))
		}
		if pixelX < 0 || pixelX >= utils.WplaceTileSize || pixelY < 0 || pixelY >= utils.WplaceTileSize {
			return followupMessage(s, i, fmt.Sprintf("❌ ピクセル座標が範囲外です: %d-%d 有効範囲: 0～999", pixelX, pixelY))
		}
		if width <= 0 || height <= 0 {
			return followupMessage(s, i, fmt.Sprintf("❌ サイズが不正です: %dx%d", width, height))
		}

		startTileX := tileX + pixelX/utils.WplaceTileSize
		startTileY := tileY + pixelY/utils.WplaceTileSize
		startPixelX := pixelX % utils.WplaceTileSize
		startPixelY := pixelY % utils.WplaceTileSize
		endPixelX := startPixelX + width
		endPixelY := startPixelY + height
		tilesX := (endPixelX + utils.WplaceTileSize - 1) / utils.WplaceTileSize
		tilesY := (endPixelY + utils.WplaceTileSize - 1) / utils.WplaceTileSize
		totalTiles := tilesX * tilesY
		if totalTiles > 10 {
			return followupMessage(s, i, fmt.Sprintf("❌ サイズが大きすぎます: %dタイル (%dx%d)", totalTiles, tilesX, tilesY))
		}
		if startTileX < 0 || startTileY < 0 || startTileX+tilesX-1 >= utils.WplaceTilesPerEdge || startTileY+tilesY-1 >= utils.WplaceTilesPerEdge {
			return followupMessage(s, i, "❌ タイル範囲が無効です。")
		}

		tilesData := make([][]byte, 0, totalTiles)
		for ty := 0; ty < tilesY; ty++ {
			for tx := 0; tx < tilesX; tx++ {
				data, err := c.downloadTile(context.Background(), startTileX+tx, startTileY+ty)
				if err != nil {
					return followupMessage(s, i, fmt.Sprintf("❌ タイル画像のダウンロードに失敗しました: %v", err))
				}
				tilesData = append(tilesData, data)
			}
		}

		combinedImg, err := combineTilesImage(tilesData, utils.WplaceTileSize, utils.WplaceTileSize, tilesX, tilesY)
		if err != nil {
			return followupMessage(s, i, fmt.Sprintf("❌ 画像結合に失敗しました: %v", err))
		}
		cropRect := image.Rect(startPixelX, startPixelY, startPixelX+width, startPixelY+height)
		cropped := combinedImg.SubImage(cropRect)
		buf := new(bytes.Buffer)
		if err := png.Encode(buf, cropped); err != nil {
			return followupMessage(s, i, fmt.Sprintf("❌ 画像エンコードに失敗しました: %v", err))
		}

		centerAbsX := float64(tileX*utils.WplaceTileSize+pixelX) + float64(width)/2.0
		centerAbsY := float64(tileY*utils.WplaceTileSize+pixelY) + float64(height)/2.0
		centerTileX := int(centerAbsX) / utils.WplaceTileSize
		centerTileY := int(centerAbsY) / utils.WplaceTileSize
		centerPixelX := int(centerAbsX) % utils.WplaceTileSize
		centerPixelY := int(centerAbsY) % utils.WplaceTileSize
		centerLatLng := utils.TilePixelToLngLat(centerTileX, centerTileY, centerPixelX, centerPixelY)
		wplaceURL := utils.BuildWplaceURL(centerLatLng.Lng, centerLatLng.Lat, calculateZoomFromWH(width, height))

		filename := fmt.Sprintf("fullsize_%d-%d-%d-%d_%dx%d.png", tileX, tileY, pixelX, pixelY, width, height)
		embed := &discordgo.MessageEmbed{
			Title: fmt.Sprintf("🗺️ フルサイズ画像: %dx%dpx", width, height),
			Color: 0x5865F2,
			Fields: []*discordgo.MessageEmbedField{
				{
					Name:   "左上座標",
					Value:  fmt.Sprintf("`%d-%d-%d-%d`", tileX, tileY, pixelX, pixelY),
					Inline: true,
				},
				{
					Name:   "サイズ",
					Value:  fmt.Sprintf("`%dx%dpx`", width, height),
					Inline: true,
				},
				{
					Name:   "使用タイル",
					Value:  fmt.Sprintf("`%dタイル (%dx%d)`", totalTiles, tilesX, tilesY),
					Inline: true,
				},
				{
					Name:   "中心座標",
					Value:  fmt.Sprintf("`%.6f, %.6f`", centerLatLng.Lng, centerLatLng.Lat),
					Inline: true,
				},
				{
					Name:   "Wplace.live",
					Value:  fmt.Sprintf("[地図で見る](%s)", wplaceURL),
					Inline: false,
				},
			},
			Image: &discordgo.MessageEmbedImage{
				URL: "attachment://" + filename,
			},
		}
		return sendImageFollowup(s, i, buf.Bytes(), filename, embed)
	}

	return respondGet(s, i, "❌ 座標またはRegion名を指定してください。coords, region, fullsize のいずれかを指定")
}

func (c *GetCommand) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        "get",
		Description: "画像やデータを取得します。",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "coords",
				Description: "タイル座標 (例: 1818-806)",
				Required:    false,
			},
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "region",
				Description: "Region名 (例: Tokyo#1)",
				Required:    false,
			},
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "fullsize",
				Description: "フルサイズ取得: 6要素 1818-806-989-358-107-142 / 8要素 1818-806-989-358-1818-806-1096-500",
				Required:    false,
			},
		},
	}
}

type Region struct {
	RegionID     int             `json:"region_id"`
	CityID       int             `json:"city_id"`
	CountryID    int             `json:"country_id"`
	RegionCoords [2]int          `json:"region_coords"`
	TileRange    RegionTileRange `json:"tile_range"`
	CenterLatLng [2]float64      `json:"center_latlng"`
	ExpectedCity string          `json:"expected_city"`
	Name         string          `json:"name"`
}

type RegionDB map[string]Region

type RegionTileRange struct {
	Min [2]int `json:"min"`
	Max [2]int `json:"max"`
}

func loadRegionDB(path string) (RegionDB, error) {
	bytes, err := os.ReadFile(path) // ioutil.ReadAll -> os.ReadFile
	if err != nil {
		return nil, err
	}
	var db RegionDB
	if err := json.Unmarshal(bytes, &db); err != nil {
		return nil, err
	}
	return db, nil
}

func respondGet(s *discordgo.Session, i *discordgo.InteractionCreate, msg string) error {
	return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: msg,
		},
	})
}

func respondDeferred(s *discordgo.Session, i *discordgo.InteractionCreate) error {
	return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})
}

func followupMessage(s *discordgo.Session, i *discordgo.InteractionCreate, msg string) error {
	_, err := s.FollowupMessageCreate(i.Interaction, false, &discordgo.WebhookParams{
		Content: msg,
	})
	return err
}

// downloadTile 単一のタイル画像をダウンロードするヘルパー関数
func (c *GetCommand) downloadTile(ctx context.Context, tlx, tly int) ([]byte, error) {
	url := fmt.Sprintf("https://backend.wplace.live/files/s0/tiles/%d/%d.png", tlx, tly)

	val, err := c.limiter.Do(ctx, "backend.wplace.live", func() (interface{}, error) {
		resp, err := http.Get(url)
		if err != nil {
			return nil, fmt.Errorf("HTTP GET failed for %s: %w", url, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("failed to download tile %d-%d (URL: %s), status: %s", tlx, tly, url, resp.Status)
		}
		return io.ReadAll(resp.Body)
	})
	if err != nil {
		return nil, err
	}
	return val.([]byte), nil
}

// sendImage 画像をDiscordに送信するヘルパー関数
func sendImage(s *discordgo.Session, i *discordgo.InteractionCreate, imageData []byte, filename string) error {
	return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Files: []*discordgo.File{
				{
					Name:        filename,
					ContentType: "image/png",
					Reader:      bytes.NewReader(imageData),
				},
			},
		},
	})
}

func sendImageWithEmbed(s *discordgo.Session, i *discordgo.InteractionCreate, imageData []byte, filename string, embed *discordgo.MessageEmbed) error {
	return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
			Files: []*discordgo.File{
				{
					Name:        filename,
					ContentType: "image/png",
					Reader:      bytes.NewReader(imageData),
				},
			},
		},
	})
}

func sendImageFollowup(s *discordgo.Session, i *discordgo.InteractionCreate, imageData []byte, filename string, embed *discordgo.MessageEmbed) error {
	params := &discordgo.WebhookParams{
		Files: []*discordgo.File{
			{
				Name:        filename,
				ContentType: "image/png",
				Reader:      bytes.NewReader(imageData),
			},
		},
	}
	if embed != nil {
		params.Embeds = []*discordgo.MessageEmbed{embed}
	}
	_, err := s.FollowupMessageCreate(i.Interaction, false, params)
	return err
}

// combineTiles 複数のタイル画像を結合するヘルパー関数
// tilesData: 各タイル画像のバイトスライス
// tileWidth, tileHeight: 単一タイルの幅と高さ (ピクセル)
// gridCols, gridRows: タイルを配置するグリッドの列数と行数
func combineTiles(tilesData [][]byte, tileWidth, tileHeight, gridCols, gridRows int) (*bytes.Buffer, error) {
	img, err := combineTilesImage(tilesData, tileWidth, tileHeight, gridCols, gridRows)
	if err != nil {
		return nil, err
	}
	buf := new(bytes.Buffer)
	if err := png.Encode(buf, img); err != nil {
		return nil, fmt.Errorf("結合画像のPNGエンコードに失敗しました: %w", err)
	}
	return buf, nil
}

func combineTilesImage(tilesData [][]byte, tileWidth, tileHeight, gridCols, gridRows int) (*image.RGBA, error) {
	if len(tilesData) == 0 {
		return nil, fmt.Errorf("結合する画像データがありません")
	}
	if len(tilesData) != gridCols*gridRows {
		return nil, fmt.Errorf("画像データの数 (%d) がグリッドサイズ (%d x %d) と一致しません", len(tilesData), gridCols, gridRows)
	}

	combinedWidth := tileWidth * gridCols
	combinedHeight := tileHeight * gridRows
	combinedImg := image.NewRGBA(image.Rect(0, 0, combinedWidth, combinedHeight))

	for i, data := range tilesData {
		tileImg, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("タイル画像のデコードに失敗しました (インデックス %d): %w", i, err)
		}

		col := i % gridCols
		row := i / gridCols

		dp := image.Pt(col*tileWidth, row*tileHeight)
		draw.Draw(combinedImg, tileImg.Bounds().Add(dp), tileImg, image.Point{}, draw.Src)
	}

	return combinedImg, nil
}

func findRegionByName(db RegionDB, name string) (Region, bool) {
	if reg, ok := db[name]; ok {
		return reg, true
	}
	for _, reg := range db {
		if strings.EqualFold(reg.Name, name) {
			return reg, true
		}
	}
	return Region{}, false
}

func calculateZoomFromWH(width, height int) float64 {
	a := 21.16849365
	bw := -0.45385241
	bh := -2.76763227
	raw := a + bw*math.Log10(float64(width)) + bh*math.Log10(float64(height))
	if raw < 10.7 {
		return 10.7
	}
	return raw
}
