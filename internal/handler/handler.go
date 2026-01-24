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
	"reflect"
	"sort"
	"strings"

	"github.com/bwmarrin/discordgo"
)

type Handler struct {
	registry *commands.Registry
	prefix   string
	botInfo  *models.BotInfo
	monitor  *monitor.Monitor
	settings *config.SettingsManager
	notifier *notifications.Notifier
}

func NewHandler(prefix string, botInfo *models.BotInfo, mon *monitor.Monitor, settingsManager *config.SettingsManager, notifier *notifications.Notifier) *Handler {
	registry := commands.NewRegistry()

	// すべてのコマンドを配列で一元管理
	var commandsList []commands.Command
	commandsList = append(commandsList,
		&commands.PingCommand{},
		commands.NewInfoCommand(botInfo),
		commands.NewStatusCommand(botInfo),
		commands.NewNowCommand(mon),
		commands.NewTimeCommand(),
		commands.NewConvertCommand(),
		commands.NewSettingsCommand(settingsManager, notifier), // settingsManager を渡す
		commands.NewGetCommand(),
		commands.NewPaintCommand(),
	)
	if mon != nil {
		commandsList = append(commandsList,
			commands.NewGraphCommand(mon),
			commands.NewTimelapseCommand(mon),
			commands.NewHeatmapCommand(mon),
		)
	}
	// HelpCommandは最後に追加し、registryを渡す
	helpCmd := commands.NewHelpCommand(registry)
	commandsList = append(commandsList, helpCmd)

	// 配列から一括登録
	for _, cmd := range commandsList {
		registry.Register(cmd)
	}

	return &Handler{
		registry: registry,
		prefix:   prefix,
		botInfo:  botInfo,
		monitor:  mon,
		settings: settingsManager, // settingsManager を使用
		notifier: notifier,
	}
}

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

func (h *Handler) SyncSlashCommands(s *discordgo.Session) error {
	log.Println("Syncing slash commands...")

	remoteCommands, err := s.ApplicationCommands(s.State.User.ID, "")
	if err != nil {
		return fmt.Errorf("could not fetch remote commands: %w", err)
	}
	localCommands := h.registry.GetSlashDefinitions()

	remoteCmdsMap := make(map[string]*discordgo.ApplicationCommand, len(remoteCommands))
	for _, cmd := range remoteCommands {
		remoteCmdsMap[cmd.Name] = cmd
	}

	for _, localCmd := range localCommands {
		remoteCmd, exists := remoteCmdsMap[localCmd.Name]
		if exists {
			if !commandsAreEqual(localCmd, remoteCmd) {
				log.Printf("Updating slash command: /%s", localCmd.Name)
				if _, err := s.ApplicationCommandEdit(s.State.User.ID, "", remoteCmd.ID, localCmd); err != nil {
					log.Printf("Failed to update command /%s: %v", localCmd.Name, err)
				}
			}
			delete(remoteCmdsMap, localCmd.Name)
		} else {
			log.Printf("Creating slash command: /%s", localCmd.Name)
			if _, err := s.ApplicationCommandCreate(s.State.User.ID, "", localCmd); err != nil {
				log.Printf("Failed to create command /%s: %v", localCmd.Name, err)
			}
		}
	}

	for _, remoteCmd := range remoteCmdsMap {
		log.Printf("Deleting outdated slash command: /%s", remoteCmd.Name)
		if err := s.ApplicationCommandDelete(s.State.User.ID, "", remoteCmd.ID); err != nil {
			log.Printf("Failed to delete command /%s: %v", remoteCmd.Name, err)
		}
	}

	log.Println("Slash command sync complete.")
	return nil
}

func (h *Handler) Cleanup(s *discordgo.Session) {
	log.Println("Skipping slash command cleanup on shutdown.")
}

func commandsAreEqual(c1, c2 *discordgo.ApplicationCommand) bool {
	if c1.Name != c2.Name || c1.Description != c2.Description {
		return false
	}
	if len(c1.Options) != len(c2.Options) {
		return false
	}

	opts1 := make([]*discordgo.ApplicationCommandOption, len(c1.Options))
	copy(opts1, c1.Options)
	sort.Slice(opts1, func(i, j int) bool { return opts1[i].Name < opts1[j].Name })

	opts2 := make([]*discordgo.ApplicationCommandOption, len(c2.Options))
	copy(opts2, c2.Options)
	sort.Slice(opts2, func(i, j int) bool { return opts2[i].Name < opts2[j].Name })

	for i := range opts1 {
		if !optionsAreEqual(opts1[i], opts2[i]) {
			return false
		}
	}
	return true
}

func optionsAreEqual(o1, o2 *discordgo.ApplicationCommandOption) bool {
	if o1.Type != o2.Type || o1.Name != o2.Name || o1.Description != o2.Description || o1.Required != o2.Required {
		return false
	}
	if len(o1.Choices) != len(o2.Choices) || len(o1.Options) != len(o2.Options) {
		return false
	}

	// Compare choices
	if len(o1.Choices) > 0 {
		// Sort choices by name for consistent comparison
		choices1 := make([]*discordgo.ApplicationCommandOptionChoice, len(o1.Choices))
		copy(choices1, o1.Choices)
		sort.Slice(choices1, func(i, j int) bool { return choices1[i].Name < choices1[j].Name })

		choices2 := make([]*discordgo.ApplicationCommandOptionChoice, len(o2.Choices))
		copy(choices2, o2.Choices)
		sort.Slice(choices2, func(i, j int) bool { return choices2[i].Name < choices2[j].Name })

		if !reflect.DeepEqual(choices1, choices2) {
			return false
		}
	}

	// Compare sub-options recursively
	if len(o1.Options) > 0 {
		subOpts1 := make([]*discordgo.ApplicationCommandOption, len(o1.Options))
		copy(subOpts1, o1.Options)
		sort.Slice(subOpts1, func(i, j int) bool { return subOpts1[i].Name < subOpts1[j].Name })

		subOpts2 := make([]*discordgo.ApplicationCommandOption, len(o2.Options))
		copy(subOpts2, o2.Options)
		sort.Slice(subOpts2, func(i, j int) bool { return subOpts2[i].Name < subOpts2[j].Name })

		for i := range subOpts1 {
			if !optionsAreEqual(subOpts1[i], subOpts2[i]) {
				return false
			}
		}
	}

	return true
}
