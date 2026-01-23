package commands

import (
	"Koukyo_discord_bot/internal/embeds"
	"Koukyo_discord_bot/internal/monitor"
	"log"

	"github.com/bwmarrin/discordgo"
)

type NowCommand struct {
	monitor *monitor.Monitor
}

func NewNowCommand(mon *monitor.Monitor) *NowCommand {
	return &NowCommand{monitor: mon}
}

func (c *NowCommand) Name() string {
	return "now"
}

func (c *NowCommand) Description() string {
	return "現在の監視状況を表示します"
}

func (c *NowCommand) ExecuteText(s *discordgo.Session, m *discordgo.MessageCreate, args []string) error {
	embed := embeds.BuildNowEmbed(c.monitor)
	_, err := s.ChannelMessageSendEmbed(m.ChannelID, embed)
	return err
}

func (c *NowCommand) ExecuteSlash(s *discordgo.Session, i *discordgo.InteractionCreate) error {
	log.Println("ExecuteSlash: /now command called")
	
	// まず即座にDeferredで応答（3秒制限回避）
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})
	if err != nil {
		log.Printf("Error in InteractionRespond: %v", err)
		return err
	}
	log.Println("Deferred response sent successfully")

	// 実際のデータ取得とEmbed生成
	log.Println("Building embed...")
	embed := embeds.BuildNowEmbed(c.monitor)
	log.Println("Embed built successfully")

	// フォローアップメッセージで送信
	log.Println("Sending follow-up message...")
	_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Embeds: &[]*discordgo.MessageEmbed{embed},
	})
	if err != nil {
		log.Printf("Error in InteractionResponseEdit: %v", err)
		return err
	}
	log.Println("Follow-up message sent successfully")
	return nil
}

func (c *NowCommand) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
	}
}
