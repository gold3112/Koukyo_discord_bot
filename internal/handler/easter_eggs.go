package handler

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"

	"github.com/bwmarrin/discordgo"
)

func (h *Handler) handleEasterEgg(s *discordgo.Session, m *discordgo.MessageCreate, cmdName string) bool {
	if h.dataDir == "" {
		return false
	}
	path := filepath.Join(h.dataDir, "easter_eggs.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	if len(data) == 0 {
		return false
	}

	var eggs map[string]string
	if err := json.Unmarshal(data, &eggs); err != nil {
		log.Printf("Failed to parse easter_eggs.json: %v", err)
		return false
	}
	reply, ok := eggs[cmdName]
	if !ok || reply == "" {
		return false
	}

	if _, err := s.ChannelMessageSend(m.ChannelID, reply); err != nil {
		log.Printf("Failed to send easter egg response: %v", err)
	}
	return true
}
