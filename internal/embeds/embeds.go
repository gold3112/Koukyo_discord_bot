package embeds

import (
	"Koukyo_discord_bot/internal/models"
	"Koukyo_discord_bot/internal/utils"
	"fmt"
	"time"

	"github.com/bwmarrin/discordgo"
)

// BuildInfoEmbed info コマンド用の埋め込みを作成
func BuildInfoEmbed(botInfo *models.BotInfo) *discordgo.MessageEmbed {
	embed := &discordgo.MessageEmbed{
		Title:       "🏯 Wplace監視テンプレート情報",
		Description: "テンプレート画像に基づく固定値です。（荒らし状況に依存しません）",
		Color:       0xFFD700, // Gold
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:   "Bot バージョン",
				Value:  botInfo.Version,
				Inline: false,
			},
			{
				Name:   "起動時刻",
				Value:  botInfo.StartTime.Format("2006-01-02 15:04:05 MST"),
				Inline: false,
			},
			{
				Name:   "稼働時間",
				Value:  formatUptime(botInfo.Uptime()),
				Inline: false,
			},
		},
		Footer: &discordgo.MessageEmbedFooter{
			Text: "Koukyo Discord Bot - Go Edition",
		},
	}
	return embed
}

// BuildTimeEmbed time コマンド用の埋め込みを作成
func BuildTimeEmbed() *discordgo.MessageEmbed {
	timezones := utils.GetCommonTimezones()
	now := time.Now()

	embed := &discordgo.MessageEmbed{
		Title: "🌍 現在時刻",
		Color: 0x3498DB, // Blue
	}

	for _, tz := range timezones {
		timeStr := utils.FormatTimeInTimezone(now, tz.Location)
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   tz.Flag + " " + tz.Label,
			Value:  timeStr,
			Inline: false,
		})
	}

	utcLoc, _ := time.LoadLocation("UTC")
	embed.Footer = &discordgo.MessageEmbedFooter{
		Text: "UTC: " + utils.FormatTimeInTimezone(now, utcLoc),
	}

	return embed
}

// BuildConvertLngLatEmbed 経度緯度 → ピクセル座標変換結果の埋め込みを作成
func BuildConvertLngLatEmbed(lng, lat float64) *discordgo.MessageEmbed {
	coord := utils.LngLatToTilePixel(lng, lat)
	url := utils.BuildWplaceURL(lng, lat, 14.76)
	hyphenCoords := utils.FormatHyphenCoords(coord)

	embed := &discordgo.MessageEmbed{
		Title:       "🗺️ 座標変換: 経度緯度 → ピクセル座標",
		Description: fmt.Sprintf("**入力:** 経度 `%.6f`, 緯度 `%.6f`", lng, lat),
		Color:       0x9B59B6, // Purple
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:   "📍 タイル座標",
				Value:  fmt.Sprintf("TlX: `%d`, TlY: `%d`", coord.TileX, coord.TileY),
				Inline: false,
			},
			{
				Name:   "🔲 ピクセル座標",
				Value:  fmt.Sprintf("PxX: `%d`, PxY: `%d`", coord.PixelX, coord.PixelY),
				Inline: false,
			},
			{
				Name:   "📋 ハイフン形式",
				Value:  fmt.Sprintf("`%s`", hyphenCoords),
				Inline: false,
			},
			{
				Name:   "🔗 Wplace URL",
				Value:  fmt.Sprintf("[地図を開く](%s)", url),
				Inline: false,
			},
		},
	}

	return embed
}

// BuildConvertPixelEmbed ピクセル座標 → 経度緯度変換結果の埋め込みを作成
func BuildConvertPixelEmbed(tileX, tileY, pixelX, pixelY int) *discordgo.MessageEmbed {
	lngLat := utils.TilePixelToLngLat(tileX, tileY, pixelX, pixelY)
	url := utils.BuildWplaceURL(lngLat.Lng, lngLat.Lat, 14.76)
	coord := &utils.Coordinate{TileX: tileX, TileY: tileY, PixelX: pixelX, PixelY: pixelY}
	hyphenCoords := utils.FormatHyphenCoords(coord)

	embed := &discordgo.MessageEmbed{
		Title: "🗺️ 座標変換: ピクセル座標 → 経度緯度",
		Description: fmt.Sprintf("**入力:** TlX `%d`, TlY `%d`, PxX `%d`, PxY `%d`",
			tileX, tileY, pixelX, pixelY),
		Color: 0x1ABC9C, // Turquoise
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:   "🌐 経度緯度",
				Value:  fmt.Sprintf("経度: `%.6f`, 緯度: `%.6f`", lngLat.Lng, lngLat.Lat),
				Inline: false,
			},
			{
				Name:   "📋 ハイフン形式",
				Value:  fmt.Sprintf("`%s`", hyphenCoords),
				Inline: false,
			},
			{
				Name:   "🔗 Wplace URL",
				Value:  fmt.Sprintf("[地図を開く](%s)", url),
				Inline: false,
			},
		},
	}

	return embed
}

// BuildNowEmbed now コマンド用の埋め込みを作成（仮実装）
func BuildNowEmbed() *discordgo.MessageEmbed {
	embed := &discordgo.MessageEmbed{
		Title:       "Wplace 監視情報",
		Description: "まだ監視データを取得できていません。",
		Color:       0x3498DB, // Blue
		Timestamp:   time.Now().Format(time.RFC3339),
	}
	embed.Footer = &discordgo.MessageEmbedFooter{
		Text: "取得時刻",
	}
	return embed
}

// formatUptime 稼働時間を人間が読みやすい形式にフォーマット
func formatUptime(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60

	if days > 0 {
		return fmt.Sprintf("%d日 %d時間 %d分 %d秒", days, hours, minutes, seconds)
	} else if hours > 0 {
		return fmt.Sprintf("%d時間 %d分 %d秒", hours, minutes, seconds)
	} else if minutes > 0 {
		return fmt.Sprintf("%d分 %d秒", minutes, seconds)
	}
	return fmt.Sprintf("%d秒", seconds)
}
