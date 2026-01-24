package notifications

import (
	"Koukyo_discord_bot/internal/activity"
	"Koukyo_discord_bot/internal/config"
	"fmt"
	"log"
	"time"

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
		embed := buildUserNotifyEmbed("🚨 新規荒らしユーザー検知", user, true)
		if _, err := n.session.ChannelMessageSendEmbed(channelID, embed); err != nil {
			log.Printf("Failed to send vandal user notification to guild %s: %v", guild.ID, err)
		}
	}
}

func buildUserNotifyEmbed(title string, user activity.UserActivity, isVandal bool) *discordgo.MessageEmbed {
	name := user.Name
	if name == "" {
		name = fmt.Sprintf("ID:%s", user.ID)
	}
	alliance := user.AllianceName
	if alliance == "" {
		alliance = "-"
	}
	count := user.VandalCount
	if !isVandal {
		count = user.RestoredCount
	}
	lastSeen := user.LastSeen
	if lastSeen == "" {
		lastSeen = "-"
	}
	embed := &discordgo.MessageEmbed{
		Title: title,
		Color: 0xE74C3C,
		Fields: []*discordgo.MessageEmbedField{
			{Name: "ユーザー", Value: name, Inline: true},
			{Name: "同盟", Value: alliance, Inline: true},
			{Name: "累計", Value: fmt.Sprintf("%d", count), Inline: true},
			{Name: "最終観測", Value: lastSeen, Inline: false},
		},
		Timestamp: time.Now().Format(time.RFC3339),
	}
	return embed
}
