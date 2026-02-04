package notifications

import (
	"fmt"

	"Koukyo_discord_bot/internal/utils"

	"github.com/bwmarrin/discordgo"
)

const (
	mainMonitorTileX  = 1818
	mainMonitorTileY  = 806
	mainMonitorPixelX = 989
	mainMonitorPixelY = 358
	mainMonitorWidth  = 107
	mainMonitorHeight = 142
)

func mainMonitorWplaceURL() string {
	centerAbsX := float64(mainMonitorTileX*utils.WplaceTileSize+mainMonitorPixelX) + float64(mainMonitorWidth)/2
	centerAbsY := float64(mainMonitorTileY*utils.WplaceTileSize+mainMonitorPixelY) + float64(mainMonitorHeight)/2
	centerTileX := int(centerAbsX) / utils.WplaceTileSize
	centerTileY := int(centerAbsY) / utils.WplaceTileSize
	centerPixelX := int(centerAbsX) % utils.WplaceTileSize
	centerPixelY := int(centerAbsY) % utils.WplaceTileSize
	center := utils.TilePixelCenterToLngLat(centerTileX, centerTileY, centerPixelX, centerPixelY)
	return utils.BuildWplaceURL(center.Lng, center.Lat, utils.ZoomFromImageSize(mainMonitorWidth, mainMonitorHeight))
}

func mainMonitorFullsizeString() string {
	return fmt.Sprintf("%d-%d-%d-%d-%d-%d", mainMonitorTileX, mainMonitorTileY, mainMonitorPixelX, mainMonitorPixelY, mainMonitorWidth, mainMonitorHeight)
}

func appendMainMonitorMapField(embed *discordgo.MessageEmbed) {
	if embed == nil {
		return
	}
	embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
		Name:   "Wplace.live",
		Value:  fmt.Sprintf("[地図で見る](%s)\n`/get fullsize:%s`", mainMonitorWplaceURL(), mainMonitorFullsizeString()),
		Inline: false,
	})
}
