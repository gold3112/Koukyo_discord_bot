package main

import (
	"Koukyo_discord_bot/internal/config"
	"Koukyo_discord_bot/internal/handler"
	"log"
	"time"

	"github.com/bwmarrin/discordgo"
)

func main() {
	cfg := config.Load()
	if cfg == nil {
		log.Fatal("Failed to load configuration")
	}
	if cfg.Token == "" {
		log.Fatal("DISCORD_TOKEN is required")
	}

	dg, err := discordgo.New("Bot " + cfg.Token)
	if err != nil {
		log.Fatal(err)
	}

	h := handler.NewHandler("!")
	dg.AddHandler(h.OnReady)
	dg.AddHandler(h.OnMessage)
	dg.AddHandler(h.OnInteractionCreate)

	err = dg.Open()
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		h.Cleanup(dg)
		dg.Close()
	}()

	log.Println("Date:", time.Now().Format("2006-01-02"))
	select {}
}
