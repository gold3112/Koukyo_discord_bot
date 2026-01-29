package commands

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"Koukyo_discord_bot/internal/activity"
	"Koukyo_discord_bot/internal/utils"

	"github.com/bwmarrin/discordgo"
)

type MeCommand struct {
	dataDir    string
	limiter    *utils.RateLimiter
	httpClient *http.Client
}

func NewMeCommand(dataDir string, limiter *utils.RateLimiter) *MeCommand {
	return &MeCommand{
		dataDir:    dataDir,
		limiter:    limiter,
		httpClient: activity.NewPixelHTTPClient(),
	}
}

func (c *MeCommand) Name() string { return "me" }

func (c *MeCommand) Description() string {
	return "自分の活動実績をカード形式で表示します"
}

func (c *MeCommand) ExecuteText(s *discordgo.Session, m *discordgo.MessageCreate, args []string) error {
	user := m.Author
	if user == nil {
		_, err := s.ChannelMessageSend(m.ChannelID, "❌ ユーザー情報を取得できませんでした。")
		return err
	}
	return c.respondMeMessage(s, m.ChannelID, user.ID, user)
}

func (c *MeCommand) ExecuteSlash(s *discordgo.Session, i *discordgo.InteractionCreate) error {
	user := interactionUser(i)
	if user == nil {
		return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "❌ ユーザー情報を取得できませんでした。",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
	}
	embed, file, err := c.buildMeEmbedByDiscordID(user.ID, user)
	if err != nil {
		return c.startLinkFlow(s, user, func(content string) error {
			return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: content,
					Flags:   discordgo.MessageFlagsEphemeral,
				},
			})
		})
	}
	return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
			Files:  buildOptionalFiles(file),
		},
	})
}

func (c *MeCommand) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
	}
}

func (c *MeCommand) respondMeMessage(s *discordgo.Session, channelID, discordID string, user *discordgo.User) error {
	embed, file, err := c.buildMeEmbedByDiscordID(discordID, user)
	if err != nil {
		return c.startLinkFlow(s, user, func(content string) error {
			_, e := s.ChannelMessageSend(channelID, content)
			return e
		})
	}
	_, err = s.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
		Embeds: []*discordgo.MessageEmbed{embed},
		Files:  buildOptionalFiles(file),
	})
	return err
}

func (c *MeCommand) buildMeEmbedByDiscordID(discordID string, user *discordgo.User) (*discordgo.MessageEmbed, *discordgo.File, error) {
	entry, err := loadUserActivityByID(c.dataDir, "", discordID)
	if err != nil {
		return nil, nil, err
	}
	embed, file := buildMeCardEmbed(entry, user)
	return embed, file, nil
}

func buildMeCardEmbed(entry userActivityEntry, user *discordgo.User) (*discordgo.MessageEmbed, *discordgo.File) {
	name := formatUserDisplayName(entry.Name, entry.ID)
	alliance := entry.Alliance
	if alliance == "" {
		alliance = "-"
	}
	discordName := entry.Discord
	if discordName == "" && user != nil {
		discordName = discordTag(user)
	}
	if discordName == "" {
		discordName = "-"
	}
	discordID := entry.DiscordID
	if discordID == "" && user != nil {
		discordID = user.ID
	}
	lastSeenText := "-"
	if !entry.LastSeen.IsZero() {
		jst := time.FixedZone("JST", 9*3600)
		lastSeenText = entry.LastSeen.In(jst).Format("2006-01-02 15:04:05")
	}
	mention := ""
	if user != nil {
		mention = fmt.Sprintf("<@%s>", user.ID)
	}

	embed := &discordgo.MessageEmbed{
		Title:       "🪪 Wplace ユーザーカード",
		Description: strings.TrimSpace(fmt.Sprintf("%s %s", mention, discordName)),
		Color:       0x5865F2,
		Fields: []*discordgo.MessageEmbedField{
			{Name: "ゲーム内ユーザー", Value: name, Inline: true},
			{Name: "同盟", Value: alliance, Inline: true},
			{Name: "Discord ID", Value: discordID, Inline: true},
			{Name: "荒らし数", Value: fmt.Sprintf("%d", entry.VandalCount), Inline: true},
			{Name: "修復数", Value: fmt.Sprintf("%d", entry.RestoredCount), Inline: true},
			{Name: "スコア", Value: fmt.Sprintf("%d", entry.Score), Inline: true},
			{Name: "最終観測", Value: lastSeenText, Inline: false},
		},
		Timestamp: time.Now().Format(time.RFC3339),
	}

	file := buildUserActivityImageFile(entry)
	if file != nil {
		embed.Image = &discordgo.MessageEmbedImage{
			URL: "attachment://" + file.Name,
		}
	}
	return embed, file
}

func discordTag(user *discordgo.User) string {
	if user == nil {
		return ""
	}
	if user.Discriminator == "0" || user.Discriminator == "" {
		return user.Username
	}
	return fmt.Sprintf("%s#%s", user.Username, user.Discriminator)
}

func interactionUser(i *discordgo.InteractionCreate) *discordgo.User {
	if i == nil {
		return nil
	}
	if i.Member != nil && i.Member.User != nil {
		return i.Member.User
	}
	if i.User != nil {
		return i.User
	}
	return nil
}

func meNotLinkedMessage(discordID string) string {
	return strings.TrimSpace(fmt.Sprintf(
		"❌ このDiscordアカウントはまだWplaceと関連付けられていません。\n"+
			"連携確認を開始します。DMを確認してください。\n"+
			"（Discord ID: %s）",
		discordID,
	))
}
