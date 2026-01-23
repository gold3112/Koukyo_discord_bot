package commands

import (
	"Koukyo_discord_bot/internal/embeds"
	"Koukyo_discord_bot/internal/models"

	"github.com/bwmarrin/discordgo"
)

type StatusCommand struct {
	botInfo *models.BotInfo
}

func NewStatusCommand(botInfo *models.BotInfo) *StatusCommand {
	return &StatusCommand{botInfo: botInfo}
}

func (c *StatusCommand) Name() string {
	return "status"
}

func (c *StatusCommand) Description() string {
	return "Botの詳細ステータスを表示します"
}

func (c *StatusCommand) ExecuteText(s *discordgo.Session, m *discordgo.MessageCreate, args []string) error {
	embed := embeds.BuildStatusEmbed(c.botInfo, s)
	_, err := s.ChannelMessageSendEmbed(m.ChannelID, embed)
	return err
}

func (c *StatusCommand) ExecuteSlash(s *discordgo.Session, i *discordgo.InteractionCreate) error {
	embed := embeds.BuildStatusEmbed(c.botInfo, s)
	return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
		},
	})
}

func (c *StatusCommand) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
	}
}
