package handler

import (
	"log"
	"github.com/bwmarrin/discordgo"
)

func OnReady(s *discordgo.Session, event *discordgo.Ready) {
	log.Println("Bot is ready!")
}

func OnMessage(s *discordgo.Session, m *discordgo.MessageCreate) {
	// Ignore bot's own messages
	if m.Author.Bot {
		return
	}
	if m.Content == "!ping" {
		s.ChannelMessageSend(m.ChannelID, "Pong!")
	}
}
