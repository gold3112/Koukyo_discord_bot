package handler

import (
	"Koukyo_discord_bot/internal/embeds"
	"log"

	"github.com/bwmarrin/discordgo"
)

func (h *Handler) OnReady(s *discordgo.Session, event *discordgo.Ready) {
	log.Println("Bot is ready!")
	log.Printf("Logged in as: %s#%s", s.State.User.Username, s.State.User.Discriminator)

	// スラッシュコマンドを同期
	if err := h.SyncSlashCommands(s); err != nil {
		log.Printf("Error syncing slash commands: %v", err)
	}

	// 各ギルドに起動情報を送信
	h.SendStartupNotification(s)
}

// SendStartupNotification 起動通知を各ギルドに送信
func (h *Handler) SendStartupNotification(s *discordgo.Session) {
	for _, guild := range s.State.Guilds {
		guildID := guild.ID
		settings := h.settings.GetGuildSettings(guildID)

		// 通知チャンネルが設定されていない場合は送信しない
		if settings.NotificationChannel == nil {
			continue
		}
		channelID := *settings.NotificationChannel

		// Bot起動通知を送信
		startupEmbed := embeds.BuildBotStartupEmbed(h.botInfo)
		_, err := s.ChannelMessageSendEmbed(channelID, startupEmbed)
		if err != nil {
			log.Printf("Error sending startup embed to guild %s: %v", guildID, err)
		}

		// 省電力モード通知（環境変数で判定）
		if h.monitor != nil && h.monitor.State.IsPowerSaveMode() {
			powerSaveEmbed := &discordgo.MessageEmbed{
				Title:       "🌙 省電力モード",
				Description: "差分率0%が継続したため、省電力モードに切り替えました。更新を一時停止しています。",
				Color:       0x888888,
				Footer:      &discordgo.MessageEmbedFooter{Text: "差分が検出されると通常運転に戻ります"},
			}
			_, err = s.ChannelMessageSendEmbed(channelID, powerSaveEmbed)
			if err != nil {
				log.Printf("Error sending power-save embed to guild %s: %v", guildID, err)
			}
			continue // 省電力モード時はnow embedは送信しない
		}

		// 現在の監視情報を送信（データがある場合）
		if h.monitor != nil && h.monitor.State.HasData() {
			nowEmbed := embeds.BuildNowEmbed(h.monitor)
			images := h.monitor.GetLatestImages()
			if images != nil && len(images.LiveImage) > 0 && len(images.DiffImage) > 0 {
				combinedImage, err2 := embeds.CombineImages(images.LiveImage, images.DiffImage)
				if err2 == nil {
					nowEmbed.Image = &discordgo.MessageEmbedImage{URL: "attachment://koukyo_combined.png"}
					_, err = s.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
						Embeds: []*discordgo.MessageEmbed{nowEmbed},
						Files: []*discordgo.File{{
							Name:   "koukyo_combined.png",
							Reader: combinedImage,
						}},
					})
				} else {
					log.Printf("Failed to combine images for startup now: %v", err2)
					_, err = s.ChannelMessageSendEmbed(channelID, nowEmbed)
				}
			} else {
				_, err = s.ChannelMessageSendEmbed(channelID, nowEmbed)
			}

			if err != nil {
				log.Printf("Error sending now embed to guild %s: %v", guildID, err)
			}
		}
	}
}
