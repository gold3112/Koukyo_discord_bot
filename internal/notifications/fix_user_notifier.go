package notifications

import (
	"Koukyo_discord_bot/internal/activity"
	"Koukyo_discord_bot/internal/config"
	"log"

	"github.com/bwmarrin/discordgo"
)

type FixUserNotifier struct {
	session  *discordgo.Session
	settings *config.SettingsManager
}

func NewFixUserNotifier(session *discordgo.Session, settings *config.SettingsManager) *FixUserNotifier {
	return &FixUserNotifier{
		session:  session,
		settings: settings,
	}
}

func (n *FixUserNotifier) Notify(user activity.UserActivity) {
	for _, guild := range n.session.State.Guilds {
		gs := n.settings.GetGuildSettings(guild.ID)
		if gs.NotificationFixChannel == nil {
			continue
		}
		channelID := *gs.NotificationFixChannel
		embed, file := buildUserNotifyEmbed("🛠️ 新規修復ユーザー検知", user, false)
		embed.Color = 0x2ECC71
		if file != nil {
			if _, err := n.session.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
				Embeds: []*discordgo.MessageEmbed{embed},
				Files:  []*discordgo.File{file},
			}); err != nil {
				log.Printf("Failed to send fix user notification to guild %s: %v", guild.ID, err)
			}
			continue
		}
		if _, err := n.session.ChannelMessageSendEmbed(channelID, embed); err != nil {
			log.Printf("Failed to send fix user notification to guild %s: %v", guild.ID, err)
		}
	}
}
