package commands

import (
	"Koukyo_discord_bot/internal/embeds"

	"github.com/bwmarrin/discordgo"
)

type TimeCommand struct{}

func NewTimeCommand() *TimeCommand {
	return &TimeCommand{}
}

func (c *TimeCommand) Name() string {
	return "time"
}

func (c *TimeCommand) Description() string {
	return "現在時刻を各タイムゾーンで表示します"
}

func (c *TimeCommand) ExecuteText(s *discordgo.Session, m *discordgo.MessageCreate, args []string) error {
	embed := embeds.BuildTimeEmbed()
	_, err := s.ChannelMessageSendEmbed(m.ChannelID, embed)
	return err
}

func (c *TimeCommand) ExecuteSlash(s *discordgo.Session, i *discordgo.InteractionCreate) error {
	embed := embeds.BuildTimeEmbed()
	return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
		},
	})
}

func (c *TimeCommand) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
	}
}
