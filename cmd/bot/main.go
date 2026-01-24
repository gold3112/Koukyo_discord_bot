package main

import (
	"Koukyo_discord_bot/internal/config"
	"Koukyo_discord_bot/internal/handler"
	"Koukyo_discord_bot/internal/models"
	"Koukyo_discord_bot/internal/monitor"
	"Koukyo_discord_bot/internal/notifications"
	"Koukyo_discord_bot/internal/version"
	"log"
	"os"
	"path/filepath"
	"time"
	_ "time/tzdata"

	"github.com/bwmarrin/discordgo"
)

// Global monitor instance
var globalMonitor *monitor.Monitor

func main() {
	cfg := config.Load()
	if cfg == nil {
		log.Fatal("Failed to load configuration")
	}
	if cfg.Token == "" {
		log.Fatal("DISCORD_TOKEN is required")
	}

	// Bot情報の初期化
	botInfo := models.NewBotInfo(version.Version)

	// 設定マネージャーの初期化
	// Dockerコンテナ内では /app/data に保存
	settingsPath := filepath.Join(".", "data", "settings.json")
	if _, err := os.Stat("/app"); err == nil {
		// Dockerコンテナ内
		settingsPath = "/app/data/settings.json"
	}
	settingsManager := config.NewSettingsManager(settingsPath)
	defer settingsManager.Close() // ここを追加
	log.Printf("Settings loaded from: %s", settingsPath)

	// WebSocket監視の開始
	powerSaveMode := os.Getenv("POWER_SAVE_MODE") == "1"
	if cfg.WebSocketURL != "" {
		globalMonitor = monitor.NewMonitor(cfg.WebSocketURL)
		if powerSaveMode {
			log.Println("Power-save mode enabled: setting PowerSaveMode on monitor state")
			globalMonitor.State.PowerSaveMode = true
		}

		if err := globalMonitor.Start(); err != nil {
			log.Printf("Failed to start monitor: %v", err)
			log.Println("Continuing without monitor...")
		} else {
			log.Printf("Monitor started: %s", cfg.WebSocketURL)
		}
	} else {
		log.Println("WEBSOCKET_URL not set, skipping monitor")
	}

	dg, err := discordgo.New("Bot " + cfg.Token)
	if err != nil {
		log.Fatal(err)
	}

	// Intentsを設定
	dg.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentsMessageContent | discordgo.IntentsGuilds

	// 通知システムの初期化
	var notifier *notifications.Notifier
	if globalMonitor != nil {
		notifier = notifications.NewNotifier(dg, globalMonitor, settingsManager)
		notifier.StartMonitoring()
		log.Println("Notification system started")
	}

	h := handler.NewHandler("!", botInfo, globalMonitor, settingsManager, notifier) // settingsManager を渡す
	dg.AddHandler(h.OnReady)
	dg.AddHandler(h.OnMessage)
	dg.AddHandler(h.OnInteractionCreate)

	err = dg.Open()
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		h.Cleanup(dg)
		if globalMonitor != nil {
			globalMonitor.Stop()
		}
		dg.Close()
	}()

	log.Printf("Bot started - Version: %s, Date: %s\n", version.Version, time.Now().Format("2006-01-02"))
	select {}
}
