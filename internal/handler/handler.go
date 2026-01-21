package handler

import (
	"Koukyo_discord_bot/internal/commands"
	"fmt"
	"log"
	"strings"

	"github.com/bwmarrin/discordgo"
)

type Handler struct {
	registry         *commands.Registry
	prefix           string
	registeredCmdIDs []string
}

func NewHandler(prefix string) *Handler {
	registry := commands.NewRegistry()
	
	// コマンド登録（テキスト＆スラッシュ両対応）
	registry.Register(&commands.PingCommand{})
	registry.Register(commands.NewHelpCommand(registry))
	
	return &Handler{
		registry:         registry,
		prefix:           prefix,
		registeredCmdIDs: []string{},
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
}

func (h *Handler) OnMessage(s *discordgo.Session, m *discordgo.MessageCreate) {
	// Botメッセージを無視
	if m.Author.Bot {
		return
	}

	// プレフィックスチェック
	if !strings.HasPrefix(m.Content, h.prefix) {
		return
	}

	// コマンドと引数をパース
	content := strings.TrimPrefix(m.Content, h.prefix)
	parts := strings.Fields(content)
	if len(parts) == 0 {
		return
	}

	cmdName := parts[0]
	args := parts[1:]

	// コマンド実行
	cmd, exists := h.registry.Get(cmdName)
	if !exists {
		return
	}

	if err := cmd.ExecuteText(s, m, args); err != nil {
		log.Printf("Error executing command %s: %v", cmdName, err)
		s.ChannelMessageSend(m.ChannelID, "An error occurred while executing the command.")
	}
}

// OnInteractionCreate スラッシュコマンドハンドラー
func (h *Handler) OnInteractionCreate(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionApplicationCommand {
		return
	}

	cmdName := i.ApplicationCommandData().Name
	cmd, exists := h.registry.Get(cmdName)
	if !exists {
		log.Printf("Unknown slash command: %s", cmdName)
		return
	}

	if err := cmd.ExecuteSlash(s, i); err != nil {
		log.Printf("Error executing slash command %s: %v", cmdName, err)
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "An error occurred while executing the command.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
	}
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
