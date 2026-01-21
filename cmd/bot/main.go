package main

import (
	"Koukyo_discord_bot/internal/config"
	"Koukyo_discord_bot/internal/handler"
	"Koukyo_discord_bot/internal/models"
	"log"
	"time"

	"github.com/bwmarrin/discordgo"
)

const BotVersion = "1.0.0-go"

func main() {
	cfg := config.Load()
	if cfg == nil {
		log.Fatal("Failed to load configuration")
	}
	if cfg.Token == "" {
		log.Fatal("DISCORD_TOKEN is required")
	}

	// Bot情報の初期化
	botInfo := models.NewBotInfo(BotVersion)

	dg, err := discordgo.New("Bot " + cfg.Token)
	if err != nil {
		log.Fatal(err)
	}

	h := handler.NewHandler("!", botInfo)
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

	log.Printf("Bot started - Version: %s, Date: %s\n", BotVersion, time.Now().Format("2006-01-02"))
	select {}
}
