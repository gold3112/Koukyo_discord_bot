package handler

import (
	"log"
	"strings"

	"github.com/bwmarrin/discordgo"
)

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
		if h.handleWatchTargetManual(s, m, cmdName) {
			return
		}
		if h.handleGetShortcut(s, m, cmdName) {
			return
		}
		if h.handleEasterEgg(s, m, cmdName) {
			return
		}
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
