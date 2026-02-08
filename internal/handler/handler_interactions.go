package handler

import (
	"Koukyo_discord_bot/internal/commands"
	"log"
	"strings"

	"github.com/bwmarrin/discordgo"
)

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
	if strings.HasPrefix(customID, "userlist:") {
		commands.HandleUserListPagination(s, i, h.dataDir)
		return
	}
	if strings.HasPrefix(customID, "useractivity:") {
		commands.HandleUserActivityPagination(s, i, h.dataDir)
		return
	}
	if customID == "useractivity_select" {
		commands.HandleUserActivitySelect(s, i, h.dataDir)
		return
	}
	if strings.HasPrefix(customID, "regionmap_page:") {
		commands.HandleRegionMapPagination(s, i)
		return
	}
	if strings.HasPrefix(customID, "regionmap_select:") {
		commands.HandleRegionMapSelect(s, i)
		return
	}
	if strings.HasPrefix(customID, "regionmap_confirm:") {
		commands.HandleRegionMapConfirm(s, i, h.limiter)
		return
	}
	if strings.HasPrefix(customID, "explanation_page:") {
		commands.HandleExplanationPagination(s, i)
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
