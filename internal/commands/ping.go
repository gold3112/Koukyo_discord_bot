package commands

import "github.com/bwmarrin/discordgo"

type PingCommand struct{}

func (c *PingCommand) Name() string {
	return "ping"
}

func (c *PingCommand) Description() string {
	return "Responds with Pong!"
}

func (c *PingCommand) ExecuteText(s *discordgo.Session, m *discordgo.MessageCreate, args []string) error {
	_, err := s.ChannelMessageSend(m.ChannelID, "Pong!")
	return err
}

func (c *PingCommand) ExecuteSlash(s *discordgo.Session, i *discordgo.InteractionCreate) error {
	return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: "Pong!",
		},
	})
}

func (c *PingCommand) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
	}
}
