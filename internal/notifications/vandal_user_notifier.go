package notifications

import (
	"Koukyo_discord_bot/internal/activity"
	"Koukyo_discord_bot/internal/config"
	"log"

	"github.com/bwmarrin/discordgo"
)

type VandalUserNotifier struct {
	session  *discordgo.Session
	settings *config.SettingsManager
}

func NewVandalUserNotifier(session *discordgo.Session, settings *config.SettingsManager) *VandalUserNotifier {
	return &VandalUserNotifier{
		session:  session,
		settings: settings,
	}
}

func (n *VandalUserNotifier) Notify(user activity.UserActivity) {
	for _, guild := range n.session.State.Guilds {
		gs := n.settings.GetGuildSettings(guild.ID)
		if gs.NotificationVandalChannel == nil {
			continue
		}
		channelID := *gs.NotificationVandalChannel
		embed, file := buildUserNotifyEmbed("🚨 新規荒らしユーザー検知", user, true)
		if file != nil {
			if _, err := n.session.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
				Embeds: []*discordgo.MessageEmbed{embed},
				Files:  []*discordgo.File{file},
			}); err != nil {
				log.Printf("Failed to send vandal user notification to guild %s: %v", guild.ID, err)
			}
			continue
		}
		if _, err := n.session.ChannelMessageSendEmbed(channelID, embed); err != nil {
			log.Printf("Failed to send vandal user notification to guild %s: %v", guild.ID, err)
		}
	}
}
