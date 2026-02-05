package handler

import "github.com/bwmarrin/discordgo"

func (h *Handler) handleProgressTargetManual(s *discordgo.Session, m *discordgo.MessageCreate, targetID string) bool {
	if h.notifier == nil {
		return false
	}
	return h.notifier.HandleProgressTargetManual(m.ChannelID, targetID)
}
