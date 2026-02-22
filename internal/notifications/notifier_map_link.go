package notifications

import (
	"fmt"

	"Koukyo_discord_bot/internal/utils"

	"github.com/bwmarrin/discordgo"
)

func mainMonitorWplaceURL() string {
	return utils.BuildMainMonitorWplaceURL()
}

func mainMonitorFullsizeString() string {
	return utils.MainMonitorFullsizeString()
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
