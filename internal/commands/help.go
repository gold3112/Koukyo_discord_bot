package commands

import (
	"fmt"
	"strings"

	"github.com/bwmarrin/discordgo"
)

type HelpCommand struct {
	registry *Registry
}

func NewHelpCommand(registry *Registry) *HelpCommand {
	return &HelpCommand{registry: registry}
}

func (c *HelpCommand) Name() string {
	return "help"
}

func (c *HelpCommand) Description() string {
	return "Shows available commands"
}

func (c *HelpCommand) ExecuteText(s *discordgo.Session, m *discordgo.MessageCreate, args []string) error {
	var sb strings.Builder
	sb.WriteString("**Available Commands:**\n")
	
	for _, cmd := range c.registry.All() {
		sb.WriteString(fmt.Sprintf("`!%s` - %s\n", cmd.Name(), cmd.Description()))
	}
	
	_, err := s.ChannelMessageSend(m.ChannelID, sb.String())
	return err
}

func (c *HelpCommand) ExecuteSlash(s *discordgo.Session, i *discordgo.InteractionCreate) error {
	var sb strings.Builder
	sb.WriteString("**Available Commands:**\n")
	
	for _, cmd := range c.registry.All() {
		sb.WriteString(fmt.Sprintf("`/%s` - %s\n", cmd.Name(), cmd.Description()))
	}
	
	return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: sb.String(),
		},
	})
}

func (c *HelpCommand) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
	}
}
