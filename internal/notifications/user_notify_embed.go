package notifications

import (
	"Koukyo_discord_bot/internal/activity"
	"bytes"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

func buildUserNotifyEmbed(title string, user activity.UserActivity, isVandal bool) (*discordgo.MessageEmbed, *discordgo.File) {
	name := formatUserDisplayName(user.Name, user.ID)
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
	discordName := user.Discord
	if discordName == "" {
		discordName = "-"
	}
	discordID := user.DiscordID
	if discordID == "" {
		discordID = "-"
	}
	embed := &discordgo.MessageEmbed{
		Title: title,
		Color: 0xE74C3C,
		Fields: []*discordgo.MessageEmbedField{
			{Name: "ユーザー", Value: name, Inline: true},
			{Name: "同盟", Value: alliance, Inline: true},
			{Name: "累計", Value: fmt.Sprintf("%d", count), Inline: true},
			{Name: "Discord", Value: discordName, Inline: true},
			{Name: "Discord ID", Value: discordID, Inline: true},
			{Name: "最終観測", Value: lastSeen, Inline: false},
		},
		Timestamp: time.Now().Format(time.RFC3339),
	}

	file := decodePictureDataURL(user.Picture)
	if file != nil {
		embed.Thumbnail = &discordgo.MessageEmbedThumbnail{
			URL: "attachment://" + file.Name,
		}
	}
	return embed, file
}

func formatUserDisplayName(name, id string) string {
	name = strings.TrimSpace(name)
	id = strings.TrimSpace(id)
	switch {
	case name != "" && id != "":
		return fmt.Sprintf("%s#%s", name, id)
	case name != "":
		return name
	case id != "":
		return fmt.Sprintf("ID:%s", id)
	default:
		return "-"
	}
}

func decodePictureDataURL(value string) *discordgo.File {
	if value == "" || !strings.HasPrefix(value, "data:image/") {
		return nil
	}
	parts := strings.SplitN(value, ",", 2)
	if len(parts) != 2 {
		return nil
	}
	header := parts[0]
	payload := parts[1]
	if !strings.Contains(header, ";base64") {
		return nil
	}
	data, err := base64.StdEncoding.DecodeString(payload)
	if err != nil || len(data) == 0 {
		return nil
	}
	ext := "png"
	switch {
	case strings.Contains(header, "image/jpeg"):
		ext = "jpg"
	case strings.Contains(header, "image/webp"):
		ext = "webp"
	}
	filename := "user_picture." + ext
	return &discordgo.File{
		Name:        filename,
		ContentType: strings.TrimPrefix(strings.SplitN(header, ";", 2)[0], "data:"),
		Reader:      bytes.NewReader(data),
	}
}
