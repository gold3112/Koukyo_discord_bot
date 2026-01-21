package commands

import (
	"Koukyo_discord_bot/internal/embeds"
	"Koukyo_discord_bot/internal/models"

	"github.com/bwmarrin/discordgo"
)

type InfoCommand struct {
	botInfo *models.BotInfo
}

func NewInfoCommand(botInfo *models.BotInfo) *InfoCommand {
	return &InfoCommand{botInfo: botInfo}
}

func (c *InfoCommand) Name() string {
	return "info"
}

func (c *InfoCommand) Description() string {
	return "Botの情報を表示します"
}

func (c *InfoCommand) ExecuteText(s *discordgo.Session, m *discordgo.MessageCreate, args []string) error {
	embed := embeds.BuildInfoEmbed(c.botInfo)
	_, err := s.ChannelMessageSendEmbed(m.ChannelID, embed)
	return err
}

func (c *InfoCommand) ExecuteSlash(s *discordgo.Session, i *discordgo.InteractionCreate) error {
	embed := embeds.BuildInfoEmbed(c.botInfo)
	return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
			Flags:  discordgo.MessageFlagsEphemeral,
		},
	})
}

func (c *InfoCommand) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
	}
}
