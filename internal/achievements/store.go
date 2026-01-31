package achievements

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type Achievement struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	AwardedAt   string `json:"awarded_at"`
}

type UserAchievements struct {
	DiscordID   string        `json:"discord_id,omitempty"`
	DiscordName string        `json:"discord_name,omitempty"`
	WplaceID    string        `json:"wplace_id,omitempty"`
	WplaceName  string        `json:"wplace_name,omitempty"`
	Achievements []Achievement `json:"achievements,omitempty"`
}

type Store struct {
	Users map[string]*UserAchievements `json:"users"`
}

func Load(path string) (*Store, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return &Store{Users: map[string]*UserAchievements{}}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return &Store{Users: map[string]*UserAchievements{}}, nil
	}
	var store Store
	if err := json.Unmarshal(data, &store); err != nil {
		return nil, err
	}
	if store.Users == nil {
		store.Users = map[string]*UserAchievements{}
	}
	return &store, nil
}

func Save(path string, store *Store) error {
	if store == nil {
		return nil
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func (s *Store) GetByDiscordID(discordID string) *UserAchievements {
	if s == nil || s.Users == nil {
		return nil
	}
	return s.Users[discordID]
}

func (s *Store) Award(discordID string, achievement Achievement) bool {
	if s == nil || discordID == "" || achievement.ID == "" {
		return false
	}
	if s.Users == nil {
		s.Users = map[string]*UserAchievements{}
	}
	user := s.Users[discordID]
	if user == nil {
		user = &UserAchievements{DiscordID: discordID}
		s.Users[discordID] = user
	}
	for _, a := range user.Achievements {
		if a.ID == achievement.ID {
			return false
		}
	}
	if achievement.AwardedAt == "" {
		achievement.AwardedAt = time.Now().UTC().Format(time.RFC3339)
	}
	user.Achievements = append(user.Achievements, achievement)
	return true
}
