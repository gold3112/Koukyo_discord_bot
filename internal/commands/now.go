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
	if c.monitor == nil {
		_, err := s.ChannelMessageSend(m.ChannelID, "❌ nowでエラーが発生しました: 監視システムが初期化されていません。")
		return err
	}
	if !c.monitor.State.HasData() {
		_, err := s.ChannelMessageSend(m.ChannelID, "❌ nowでエラーが発生しました: 監視データがまだ受信できていません。")
		return err
	}
	embed := embeds.BuildNowEmbed(c.monitor)

	// 画像データを取得
	images := c.monitor.GetLatestImages()
	if images != nil && len(images.LiveImage) > 0 && len(images.DiffImage) > 0 {
		// 画像結合（Live + Diff）
		combinedImage, err := embeds.CombineImages(images.LiveImage, images.DiffImage)
		if err != nil {
			log.Printf("Failed to combine images: %v", err)
			// 画像なしで送信
			_, err = s.ChannelMessageSendEmbed(m.ChannelID, embed)
			return err
		}

		// 画像付きで送信
		embed.Image = &discordgo.MessageEmbedImage{
			URL: "attachment://koukyo_combined.png",
		}

		_, err = s.ChannelMessageSendComplex(m.ChannelID, &discordgo.MessageSend{
			Embeds: []*discordgo.MessageEmbed{embed},
			Files: []*discordgo.File{
				{
					Name:   "koukyo_combined.png",
					Reader: combinedImage,
				},
			},
		})
		return err
	}

	// 画像がない場合は通常のEmbedのみ
	_, err := s.ChannelMessageSendEmbed(m.ChannelID, embed)
	return err
}

func (c *NowCommand) ExecuteSlash(s *discordgo.Session, i *discordgo.InteractionCreate) error {
	log.Println("ExecuteSlash: /now command called")
	if c.monitor == nil {
		return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "❌ nowでエラーが発生しました: 監視システムが初期化されていません。",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
	}
	if !c.monitor.State.HasData() {
		return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "❌ nowでエラーが発生しました: 監視データがまだ受信できていません。",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
	}

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

	// 画像データを取得
	images := c.monitor.GetLatestImages()
	if images != nil && len(images.LiveImage) > 0 && len(images.DiffImage) > 0 {
		// 画像結合（Live + Diff）
		combinedImage, err := embeds.CombineImages(images.LiveImage, images.DiffImage)
		if err != nil {
			log.Printf("Failed to combine images: %v", err)
			// 画像なしで送信
			_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
				Embeds: &[]*discordgo.MessageEmbed{embed},
			})
			return err
		}

		// 画像付きで送信
		embed.Image = &discordgo.MessageEmbedImage{
			URL: "attachment://koukyo_combined.png",
		}

		log.Println("Sending follow-up message with image...")
		_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Embeds: &[]*discordgo.MessageEmbed{embed},
			Files: []*discordgo.File{
				{
					Name:   "koukyo_combined.png",
					Reader: combinedImage,
				},
			},
		})
		if err != nil {
			log.Printf("Error in InteractionResponseEdit: %v", err)
			return err
		}
		log.Println("Follow-up message with image sent successfully")
		return nil
	}

	// 画像がない場合は通常のEmbedのみ
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
