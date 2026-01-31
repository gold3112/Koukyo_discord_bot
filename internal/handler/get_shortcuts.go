package handler

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"Koukyo_discord_bot/internal/commands"

	"github.com/bwmarrin/discordgo"
)

const getShortcutCacheTTL = 1 * time.Minute

type getShortcutConfig struct {
	Fullsize string
	Label    string
	Aliases  []string
}

type getShortcutCache struct {
	mu        sync.Mutex
	data      map[string]getShortcutConfig
	fetchedAt time.Time
}

var globalGetShortcutCache getShortcutCache

func (h *Handler) handleGetShortcut(s *discordgo.Session, m *discordgo.MessageCreate, cmdName string) bool {
	if h.dataDir == "" {
		return false
	}
	cfg, ok := loadGetShortcutConfig(h.dataDir, cmdName)
	if !ok || cfg.Fullsize == "" {
		return false
	}
	getCmd := commands.NewGetCommand(h.limiter)
	if err := getCmd.ExecuteFullsizeText(s, m, cfg.Fullsize, cfg.Label); err != nil {
		log.Printf("Failed to execute get shortcut %s: %v", cmdName, err)
	}
	return true
}

func loadGetShortcutConfig(dataDir, cmdName string) (getShortcutConfig, bool) {
	configs, ok := loadGetShortcutConfigs(dataDir)
	if !ok {
		return getShortcutConfig{}, false
	}
	if cfg, ok := configs[cmdName]; ok {
		return cfg, true
	}
	for _, cfg := range configs {
		if hasShortcutAlias(cfg.Aliases, cmdName) {
			return cfg, true
		}
	}
	return getShortcutConfig{}, false
}

func loadGetShortcutConfigs(dataDir string) (map[string]getShortcutConfig, bool) {
	globalGetShortcutCache.mu.Lock()
	defer globalGetShortcutCache.mu.Unlock()

	if globalGetShortcutCache.data != nil && time.Since(globalGetShortcutCache.fetchedAt) < getShortcutCacheTTL {
		return globalGetShortcutCache.data, true
	}

	path := filepath.Join(dataDir, "get_shortcuts.json")
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return nil, false
	}

	raw := make(map[string]json.RawMessage)
	if err := json.Unmarshal(data, &raw); err != nil {
		log.Printf("Failed to parse get_shortcuts.json: %v", err)
		return nil, false
	}

	out := make(map[string]getShortcutConfig, len(raw))
	for key, blob := range raw {
		cfg := getShortcutConfig{}
		if err := cfg.UnmarshalJSON(blob); err != nil {
			log.Printf("Failed to parse get_shortcuts.json entry %s: %v", key, err)
			continue
		}
		out[key] = cfg
	}
	if len(out) == 0 {
		return nil, false
	}
	globalGetShortcutCache.data = out
	globalGetShortcutCache.fetchedAt = time.Now()
	return out, true
}

func (c *getShortcutConfig) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		c.Fullsize = s
		return nil
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if v, ok := raw["fullsize"]; ok {
		_ = json.Unmarshal(v, &c.Fullsize)
	}
	if v, ok := raw["label"]; ok {
		_ = json.Unmarshal(v, &c.Label)
	}
	if v, ok := raw["aliases"]; ok {
		_ = json.Unmarshal(v, &c.Aliases)
	}
	return nil
}

func hasShortcutAlias(aliases []string, cmdName string) bool {
	for _, alias := range aliases {
		if strings.EqualFold(alias, cmdName) {
			return true
		}
	}
	return false
}
