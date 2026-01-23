package handler

import (
	"Koukyo_discord_bot/internal/commands"
	"Koukyo_discord_bot/internal/config"
	"Koukyo_discord_bot/internal/embeds"
	"Koukyo_discord_bot/internal/models"
	"Koukyo_discord_bot/internal/monitor"
	"Koukyo_discord_bot/internal/notifications"
	"fmt"
	"log"
	"strings"

	"github.com/bwmarrin/discordgo"
)

type Handler struct {
	registry         *commands.Registry
	prefix           string
	registeredCmdIDs []string
	botInfo          *models.BotInfo
	monitor          *monitor.Monitor
	settings         *config.SettingsManager
	notifier         *notifications.Notifier
}

func NewHandler(prefix string, botInfo *models.BotInfo, mon *monitor.Monitor, settings *config.SettingsManager, notifier *notifications.Notifier) *Handler {
	registry := commands.NewRegistry()

	// コマンド登録（テキスト＆スラッシュ両対応）
	registry.Register(&commands.PingCommand{})
	registry.Register(commands.NewHelpCommand(registry))
	registry.Register(commands.NewInfoCommand(botInfo))
	registry.Register(commands.NewStatusCommand(botInfo))
	registry.Register(commands.NewNowCommand(mon))
	registry.Register(commands.NewTimeCommand())
	registry.Register(commands.NewConvertCommand())
	registry.Register(commands.NewSettingsCommand(settings, notifier))
	if mon != nil {
		registry.Register(commands.NewGraphCommand(mon))
		registry.Register(commands.NewTimelapseCommand(mon))
		registry.Register(commands.NewHeatmapCommand(mon))
	}

	return &Handler{
		registry:         registry,
		prefix:           prefix,
		registeredCmdIDs: []string{},
		botInfo:          botInfo,
		monitor:          mon,
		settings:         settings,
		notifier:         notifier,
	}
}

func (h *Handler) OnReady(s *discordgo.Session, event *discordgo.Ready) {
	log.Println("Bot is ready!")
	log.Printf("Logged in as: %s#%s", s.State.User.Username, s.State.User.Discriminator)

	// スラッシュコマンドを同期
	if err := h.SyncSlashCommands(s); err != nil {
		log.Printf("Error syncing slash commands: %v", err)
	} else {
		log.Println("Slash commands synced successfully")
	}

	// 各ギルドに起動情報を送信
	h.SendStartupNotification(s)
}

// SendStartupNotification 起動通知を各ギルドに送信
func (h *Handler) SendStartupNotification(s *discordgo.Session) {
	for _, guild := range s.State.Guilds {
		guildID := guild.ID
		settings := h.settings.GetGuildSettings(guildID)

		// 通知チャンネルが設定されている場合、そこに送信
		var channelID string
		if settings.NotificationChannel != nil {
			channelID = *settings.NotificationChannel
		} else {
			// 通知チャンネルが設定されていない場合は、最初のテキストチャンネルに送信
			channels, err := s.GuildChannels(guildID)
			if err != nil {
				log.Printf("Error fetching guild channels for %s: %v", guildID, err)
				continue
			}

			found := false
			for _, ch := range channels {
				if ch.Type == discordgo.ChannelTypeGuildText {
					channelID = ch.ID
					found = true
					break
				}
			}

			if !found {
				log.Printf("No text channel found for guild %s", guildID)
				continue
			}
		}

		// Bot起動通知を送信
		startupEmbed := embeds.BuildBotStartupEmbed(h.botInfo)
		_, err := s.ChannelMessageSendEmbed(channelID, startupEmbed)
		if err != nil {
			log.Printf("Error sending startup embed to guild %s: %v", guildID, err)
		}

		// 省電力モード通知（環境変数で判定）
		if h.monitor != nil && h.monitor.State.PowerSaveMode {
			powerSaveEmbed := &discordgo.MessageEmbed{
				Title:       "🌙 省電力モード",
				Description: "差分率0%が継続したため、省電力モードに切り替える再起動を行いました。更新を一時停止しています。",
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

func (h *Handler) OnMessage(s *discordgo.Session, m *discordgo.MessageCreate) {
	// Botメッセージを無視
	if m.Author.Bot {
		return
	}

	log.Printf("Message received: '%s' from %s", m.Content, m.Author.Username)

	// プレフィックスチェック
	if !strings.HasPrefix(m.Content, h.prefix) {
		return
	}

	log.Println("Prefix matched!")

	// コマンドと引数をパース
	content := strings.TrimPrefix(m.Content, h.prefix)
	parts := strings.Fields(content)
	if len(parts) == 0 {
		return
	}

	cmdName := parts[0]
	args := parts[1:]

	log.Printf("Parsed command: '%s', args: %v", cmdName, args)

	// コマンド実行
	cmd, exists := h.registry.Get(cmdName)
	if !exists {
		log.Printf("Command '%s' not found in registry", cmdName)
		return
	}

	log.Printf("Executing text command: %s", cmdName)
	if err := cmd.ExecuteText(s, m, args); err != nil {
		log.Printf("Error executing command %s: %v", cmdName, err)
		s.ChannelMessageSend(m.ChannelID, "An error occurred while executing the command.")
	} else {
		log.Printf("Command %s completed successfully", cmdName)
	}
}

// OnInteractionCreate スラッシュコマンド・ボタン・モーダルハンドラー
func (h *Handler) OnInteractionCreate(s *discordgo.Session, i *discordgo.InteractionCreate) {
	log.Printf("Interaction received: Type=%d", i.Type)

	switch i.Type {
	case discordgo.InteractionApplicationCommand:
		// スラッシュコマンド
		h.handleSlashCommand(s, i)
	case discordgo.InteractionMessageComponent:
		// ボタンやセレクトメニュー
		h.handleMessageComponent(s, i)
	case discordgo.InteractionModalSubmit:
		// モーダル送信
		h.handleModalSubmit(s, i)
	default:
		log.Printf("Unknown interaction type: %d", i.Type)
	}
}

func (h *Handler) handleSlashCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	cmdName := i.ApplicationCommandData().Name
	log.Printf("Slash command: /%s", cmdName)

	cmd, exists := h.registry.Get(cmdName)
	if !exists {
		log.Printf("Unknown slash command: %s", cmdName)
		return
	}

	log.Printf("Executing slash command: /%s", cmdName)
	if err := cmd.ExecuteSlash(s, i); err != nil {
		log.Printf("Error executing slash command %s: %v", cmdName, err)
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "An error occurred while executing the command.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
	} else {
		log.Printf("Slash command %s completed", cmdName)
	}
}

func (h *Handler) handleMessageComponent(s *discordgo.Session, i *discordgo.InteractionCreate) {
	customID := i.MessageComponentData().CustomID
	log.Printf("Message component: %s", customID)

	// 設定パネルのボタン
	if strings.HasPrefix(customID, "settings_") {
		commands.HandleSettingsButtonInteraction(s, i, h.settings, h.notifier)
		return
	}

	// 設定パネルのセレクトメニュー
	if strings.HasPrefix(customID, "select_") {
		commands.HandleSettingsSelectMenu(s, i, h.settings)
		return
	}

	log.Printf("Unknown message component: %s", customID)
}

func (h *Handler) handleModalSubmit(s *discordgo.Session, i *discordgo.InteractionCreate) {
	modalID := i.ModalSubmitData().CustomID
	log.Printf("Modal submit: %s", modalID)

	// 設定モーダル
	if strings.HasPrefix(modalID, "modal_set_") {
		commands.HandleSettingsModalSubmit(s, i, h.settings, h.notifier)
		return
	}

	log.Printf("Unknown modal: %s", modalID)
}

// SyncSlashCommands スラッシュコマンドをDiscordに同期
func (h *Handler) SyncSlashCommands(s *discordgo.Session) error {
	// 既存のコマンドを削除
	for _, cmdID := range h.registeredCmdIDs {
		if err := s.ApplicationCommandDelete(s.State.User.ID, "", cmdID); err != nil {
			log.Printf("Failed to delete command %s: %v", cmdID, err)
		}
	}
	h.registeredCmdIDs = []string{}

	// 新しいコマンドを登録
	definitions := h.registry.GetSlashDefinitions()
	for _, def := range definitions {
		cmd, err := s.ApplicationCommandCreate(s.State.User.ID, "", def)
		if err != nil {
			return fmt.Errorf("failed to create command %s: %w", def.Name, err)
		}
		h.registeredCmdIDs = append(h.registeredCmdIDs, cmd.ID)
		log.Printf("Registered slash command: /%s", def.Name)
	}

	return nil
}

// Cleanup スラッシュコマンドのクリーンアップ
func (h *Handler) Cleanup(s *discordgo.Session) {
	log.Println("Cleaning up slash commands...")
	for _, cmdID := range h.registeredCmdIDs {
		if err := s.ApplicationCommandDelete(s.State.User.ID, "", cmdID); err != nil {
			log.Printf("Failed to delete command %s: %v", cmdID, err)
		}
	}
}
