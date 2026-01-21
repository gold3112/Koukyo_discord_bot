package commands

import (
	"Koukyo_discord_bot/internal/embeds"

	"github.com/bwmarrin/discordgo"
)

type NowCommand struct{}

func NewNowCommand() *NowCommand {
	return &NowCommand{}
}

func (c *NowCommand) Name() string {
	return "now"
}

func (c *NowCommand) Description() string {
	return "現在の監視状況を表示します"
}

func (c *NowCommand) ExecuteText(s *discordgo.Session, m *discordgo.MessageCreate, args []string) error {
	embed := embeds.BuildNowEmbed()
	_, err := s.ChannelMessageSendEmbed(m.ChannelID, embed)
	return err
}

func (c *NowCommand) ExecuteSlash(s *discordgo.Session, i *discordgo.InteractionCreate) error {
	embed := embeds.BuildNowEmbed()
	return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
		},
	})
}

func (c *NowCommand) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
	}
}
