package handler

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"

	"Koukyo_discord_bot/internal/eastereggs"

	"github.com/bwmarrin/discordgo"
)

func (h *Handler) handleEasterEgg(s *discordgo.Session, m *discordgo.MessageCreate, cmdName string) bool {
	if reply, ok := eastereggs.HandleSleepyHeresy(m.Content, m.GuildID, m.Author.ID); ok {
		if _, err := s.ChannelMessageSend(m.ChannelID, reply); err != nil {
			log.Printf("Failed to send easter egg response: %v", err)
		}
		return true
	}
	if replies, ok := eastereggs.HandleSleepyboard(cmdName, m.GuildID, m.Author.ID); ok {
		for _, reply := range replies {
			if _, err := s.ChannelMessageSend(m.ChannelID, reply); err != nil {
				log.Printf("Failed to send easter egg response: %v", err)
			}
		}
		return true
	}
	if reply, ok := eastereggs.RandomReply(cmdName); ok {
		if _, err := s.ChannelMessageSend(m.ChannelID, reply); err != nil {
			log.Printf("Failed to send easter egg response: %v", err)
		}
		return true
	}
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

	var eggs map[string]eggConfig
	if err := json.Unmarshal(data, &eggs); err != nil {
		log.Printf("Failed to parse easter_eggs.json: %v", err)
		return false
	}
	cfg, ok := eggs[cmdName]
	if !ok {
		for _, candidate := range eggs {
			if len(candidate.Aliases) == 0 {
				continue
			}
			if hasAlias(candidate.Aliases, cmdName) {
				cfg = candidate
				ok = true
				break
			}
		}
	}
	if !ok || (cfg.Reply == "" && len(cfg.Choices) == 0) {
		return false
	}

	if !eastereggs.ShouldTriggerChance(cfg.ChancePercent) {
		return false
	}

	reply := cfg.Reply
	if len(cfg.Choices) > 0 {
		chosen, ok := eastereggs.WeightedChoice(cfg.Choices)
		if !ok || chosen == "" {
			return false
		}
		reply = chosen
	}

	if _, err := s.ChannelMessageSend(m.ChannelID, reply); err != nil {
		log.Printf("Failed to send easter egg response: %v", err)
	}
	return true
}

type eggConfig struct {
	Reply         string
	ChancePercent float64
	Choices       []eastereggs.WeightedReply
	Aliases       []string
}

func (c *eggConfig) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		c.Reply = s
		c.ChancePercent = 100
		return nil
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	c.ChancePercent = 100
	if v, ok := raw["reply"]; ok {
		_ = json.Unmarshal(v, &c.Reply)
	}
	if v, ok := raw["chance"]; ok {
		_ = json.Unmarshal(v, &c.ChancePercent)
	}
	if v, ok := raw["choices"]; ok {
		_ = json.Unmarshal(v, &c.Choices)
	}
	if v, ok := raw["aliases"]; ok {
		_ = json.Unmarshal(v, &c.Aliases)
	}
	return nil
}

func hasAlias(aliases []string, cmdName string) bool {
	for _, alias := range aliases {
		if alias == cmdName {
			return true
		}
	}
	return false
}
