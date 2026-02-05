package handler

import (
	"github.com/bwmarrin/discordgo"
)

func (h *Handler) handleWatchTargetManual(s *discordgo.Session, m *discordgo.MessageCreate, targetID string) bool {
	if h.notifier == nil {
		return false
	}
	return h.notifier.HandleWatchTargetManual(m.ChannelID, targetID)
}
