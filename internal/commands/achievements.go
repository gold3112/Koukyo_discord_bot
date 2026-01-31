package commands

import (
	"Koukyo_discord_bot/internal/achievements"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

type AchievementsCommand struct {
	dataDir string
}

func NewAchievementsCommand(dataDir string) *AchievementsCommand {
	return &AchievementsCommand{dataDir: dataDir}
}

func (c *AchievementsCommand) Name() string { return "achievements" }
func (c *AchievementsCommand) Description() string {
	return "自分の実績一覧を表示します"
}

func (c *AchievementsCommand) ExecuteText(s *discordgo.Session, m *discordgo.MessageCreate, args []string) error {
	user := m.Author
	if user == nil {
		_, err := s.ChannelMessageSend(m.ChannelID, "❌ ユーザー情報を取得できませんでした。")
		return err
	}
	embed, err := c.buildAchievementsEmbed(user.ID, discordTag(user))
	if err != nil {
		_, e := s.ChannelMessageSend(m.ChannelID, "❌ "+err.Error())
		return e
	}
	_, err = s.ChannelMessageSendEmbed(m.ChannelID, embed)
	return err
}

func (c *AchievementsCommand) ExecuteSlash(s *discordgo.Session, i *discordgo.InteractionCreate) error {
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
	embed, err := c.buildAchievementsEmbed(user.ID, discordTag(user))
	if err != nil {
		return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "❌ " + err.Error(),
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
	}
	return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
			Flags:  discordgo.MessageFlagsEphemeral,
		},
	})
}

func (c *AchievementsCommand) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
	}
}

func (c *AchievementsCommand) buildAchievementsEmbed(discordID, displayName string) (*discordgo.MessageEmbed, error) {
	if c.dataDir == "" {
		return nil, fmt.Errorf("dataDir is empty")
	}
	storePath := filepath.Join(c.dataDir, "achievements.json")
	store, err := achievements.Load(storePath)
	if err != nil {
		return nil, err
	}
	user := store.GetByDiscordID(discordID)
	titleName := displayName
	if titleName == "" {
		titleName = discordID
	}

	embed := &discordgo.MessageEmbed{
		Title:       "🏅 実績一覧",
		Description: fmt.Sprintf("ユーザー: %s", titleName),
		Color:       0xF1C40F,
		Timestamp:   time.Now().Format(time.RFC3339),
	}

	if user == nil || len(user.Achievements) == 0 {
		embed.Fields = []*discordgo.MessageEmbedField{
			{
				Name:  "取得済み実績",
				Value: "まだ実績はありません。",
			},
		}
		return embed, nil
	}

	achievementsList := make([]achievements.Achievement, 0, len(user.Achievements))
	achievementsList = append(achievementsList, user.Achievements...)
	sort.SliceStable(achievementsList, func(i, j int) bool {
		return achievementsList[i].AwardedAt > achievementsList[j].AwardedAt
	})

	lines := make([]string, 0, len(achievementsList))
	for _, a := range achievementsList {
		line := "• " + a.Name
		if a.Description != "" {
			line += fmt.Sprintf(" — %s", a.Description)
		}
		if a.AwardedAt != "" {
			if t, err := time.Parse(time.RFC3339, a.AwardedAt); err == nil {
				line += fmt.Sprintf(" (%s)", t.In(time.FixedZone("JST", 9*3600)).Format("2006-01-02"))
			}
		}
		lines = append(lines, line)
	}

	embed.Fields = []*discordgo.MessageEmbedField{
		{
			Name:  fmt.Sprintf("取得済み実績 (%d)", len(lines)),
			Value: strings.Join(lines, "\n"),
		},
	}
	return embed, nil
}
