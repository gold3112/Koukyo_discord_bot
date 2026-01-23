package config

import "os"

type Config struct {
	Token        string
	WebSocketURL string
}

func Load() *Config {
	return &Config{
		Token:        os.Getenv("DISCORD_TOKEN"),
		WebSocketURL: os.Getenv("WEBSOCKET_URL"),
	}
}
