package commands

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/png"
	"strconv"
	"strings"
	"time"

	"Koukyo_discord_bot/internal/utils"

	"github.com/bwmarrin/discordgo"
)

func parseFullsizeString(fullsize string) (tileX, tileY, pixelX, pixelY, width, height int, err error) {
	parts := strings.Split(fullsize, "-")
	switch len(parts) {
	case 6:
		var convErr error
		tileX, convErr = strconv.Atoi(parts[0])
		if convErr != nil {
			return 0, 0, 0, 0, 0, 0, fmt.Errorf("座標値が不正です: %s", fullsize)
		}
		tileY, convErr = strconv.Atoi(parts[1])
		if convErr != nil {
			return 0, 0, 0, 0, 0, 0, fmt.Errorf("座標値が不正です: %s", fullsize)
		}
		pixelX, convErr = strconv.Atoi(parts[2])
		if convErr != nil {
			return 0, 0, 0, 0, 0, 0, fmt.Errorf("座標値が不正です: %s", fullsize)
		}
		pixelY, convErr = strconv.Atoi(parts[3])
		if convErr != nil {
			return 0, 0, 0, 0, 0, 0, fmt.Errorf("座標値が不正です: %s", fullsize)
		}
		width, convErr = strconv.Atoi(parts[4])
		if convErr != nil {
			return 0, 0, 0, 0, 0, 0, fmt.Errorf("サイズが不正です: %s", fullsize)
		}
		height, convErr = strconv.Atoi(parts[5])
		if convErr != nil {
			return 0, 0, 0, 0, 0, 0, fmt.Errorf("サイズが不正です: %s", fullsize)
		}
	case 8:
		values := make([]int, 0, 8)
		for _, part := range parts {
			val, convErr := strconv.Atoi(part)
			if convErr != nil {
				return 0, 0, 0, 0, 0, 0, fmt.Errorf("座標値が不正です: %s", fullsize)
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
		return 0, 0, 0, 0, 0, 0, fmt.Errorf("fullsize形式が正しくありません: %s", fullsize)
	}
	return tileX, tileY, pixelX, pixelY, width, height, nil
}

func (c *GetCommand) ExecuteFullsizeText(s *discordgo.Session, m *discordgo.MessageCreate, fullsize, label string) error {
	tileX, tileY, pixelX, pixelY, width, height, err := parseFullsizeString(fullsize)
	if err != nil {
		_, e := s.ChannelMessageSend(m.ChannelID, "❌ "+err.Error())
		return e
	}
	if tileX < 0 || tileX >= utils.WplaceTilesPerEdge || tileY < 0 || tileY >= utils.WplaceTilesPerEdge {
		_, e := s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("❌ タイル座標が範囲外です: %d-%d 有効範囲: 0～2047", tileX, tileY))
		return e
	}
	if pixelX < 0 || pixelX >= utils.WplaceTileSize || pixelY < 0 || pixelY >= utils.WplaceTileSize {
		_, e := s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("❌ ピクセル座標が範囲外です: %d-%d 有効範囲: 0～999", pixelX, pixelY))
		return e
	}
	if width <= 0 || height <= 0 {
		_, e := s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("❌ サイズが不正です: %dx%d", width, height))
		return e
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
		_, e := s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("❌ サイズが大きすぎます: %dタイル (%dx%d)", totalTiles, tilesX, tilesY))
		return e
	}
	if startTileX < 0 || startTileY < 0 || startTileX+tilesX-1 >= utils.WplaceTilesPerEdge || startTileY+tilesY-1 >= utils.WplaceTilesPerEdge {
		_, e := s.ChannelMessageSend(m.ChannelID, "❌ タイル範囲が無効です。")
		return e
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	tilesData, err := c.downloadTilesGrid(ctx, startTileX, startTileY, tilesX, tilesY)
	cancel()
	if err != nil {
		_, e := s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("❌ タイル画像のダウンロードに失敗しました: %v", err))
		return e
	}

	cropRect := image.Rect(startPixelX, startPixelY, startPixelX+width, startPixelY+height)
	cropped, err := combineTilesCropped(tilesData, utils.WplaceTileSize, utils.WplaceTileSize, tilesX, tilesY, cropRect)
	if err != nil {
		_, e := s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("❌ 画像結合に失敗しました: %v", err))
		return e
	}
	buf := new(bytes.Buffer)
	if err := png.Encode(buf, cropped); err != nil {
		_, e := s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("❌ 画像エンコードに失敗しました: %v", err))
		return e
	}

	centerAbsX := float64(tileX*utils.WplaceTileSize+pixelX) + float64(width)/2.0
	centerAbsY := float64(tileY*utils.WplaceTileSize+pixelY) + float64(height)/2.0
	centerTileX := int(centerAbsX) / utils.WplaceTileSize
	centerTileY := int(centerAbsY) / utils.WplaceTileSize
	centerPixelX := int(centerAbsX) % utils.WplaceTileSize
	centerPixelY := int(centerAbsY) % utils.WplaceTileSize
	centerLatLng := utils.TilePixelCenterToLngLat(centerTileX, centerTileY, centerPixelX, centerPixelY)
	wplaceURL := utils.BuildWplaceURL(centerLatLng.Lng, centerLatLng.Lat, calculateZoomFromWH(width, height))

	filename := fmt.Sprintf("fullsize_%d-%d-%d-%d_%dx%d.png", tileX, tileY, pixelX, pixelY, width, height)
	title := fmt.Sprintf("🗺️ フルサイズ画像: %dx%dpx", width, height)
	if label != "" {
		title = "🗺️ " + label
	}
	embed := &discordgo.MessageEmbed{
		Title: title,
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
	_, sendErr := s.ChannelMessageSendComplex(m.ChannelID, &discordgo.MessageSend{
		Embeds: []*discordgo.MessageEmbed{embed},
		Files: []*discordgo.File{
			{
				Name:        filename,
				ContentType: "image/png",
				Reader:      bytes.NewReader(buf.Bytes()),
			},
		},
	})
	return sendErr
}
