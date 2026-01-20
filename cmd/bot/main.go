package main

import (
	"Koukyo_discord_bot/internal/config"
	"Koukyo_discord_bot/internal/handler"
	"log"

	"github.com/bwmarrin/discordgo"
)

func main() {
	cfg := config.Load()
	if cfg.Token == "" {
		log.Fatal("DISCORD_TOKEN is required")
	}

	dg, err := discordgo.New("Bot " + cfg.Token)
	if err != nil {
		log.Fatal(err)
	}

	dg.AddHandler(handler.OnReady)
	dg.AddHandler(handler.OnMessage)

	err = dg.Open()
	if err != nil {
		log.Fatal(err)
	}
	defer dg.Close()

	log.Println("Bot is running")
	select {}
}
