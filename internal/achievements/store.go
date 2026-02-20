package achievements

import (
	"Koukyo_discord_bot/internal/utils"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

type Achievement struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	AwardedAt   string `json:"awarded_at"`
}

type UserAchievements struct {
	DiscordID    string        `json:"discord_id,omitempty"`
	DiscordName  string        `json:"discord_name,omitempty"`
	WplaceID     string        `json:"wplace_id,omitempty"`
	WplaceName   string        `json:"wplace_name,omitempty"`
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
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	return utils.WriteFileAtomic(path, data)
}

func (s *Store) GetByDiscordID(discordID string) *UserAchievements {
	if s == nil || s.Users == nil {
		return nil
	}
	if discordID == "" {
		return nil
	}
	if user, ok := s.Users[discordID]; ok {
		return user
	}
	return nil
}

func (s *Store) GetByWplaceID(wplaceID string) *UserAchievements {
	if s == nil || s.Users == nil || wplaceID == "" {
		return nil
	}
	return s.Users[wplaceIdentityKey(wplaceID)]
}

func (s *Store) GetByIdentity(discordID, wplaceID string) *UserAchievements {
	if s == nil || s.Users == nil {
		return nil
	}
	if discordID != "" {
		if user, ok := s.Users[discordID]; ok {
			return user
		}
	}
	if wplaceID != "" {
		if user, ok := s.Users[wplaceIdentityKey(wplaceID)]; ok {
			return user
		}
	}
	return nil
}

func (s *Store) Award(discordID string, achievement Achievement) bool {
	return s.AwardByIdentity(discordID, "", achievement)
}

func (s *Store) AwardByIdentity(discordID, wplaceID string, achievement Achievement) bool {
	key := resolveIdentityKey(discordID, wplaceID)
	if s == nil || key == "" || achievement.ID == "" {
		return false
	}
	if s.Users == nil {
		s.Users = map[string]*UserAchievements{}
	}
	user := s.Users[key]
	if user == nil {
		user = &UserAchievements{
			DiscordID: discordID,
			WplaceID:  wplaceID,
		}
		s.Users[key] = user
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

func (s *Store) UpsertUserProfile(discordID, discordName, wplaceID, wplaceName string) {
	key := resolveIdentityKey(discordID, wplaceID)
	if s == nil || key == "" {
		return
	}
	if s.Users == nil {
		s.Users = map[string]*UserAchievements{}
	}
	user := s.Users[key]
	if user == nil {
		user = &UserAchievements{
			DiscordID: discordID,
			WplaceID:  wplaceID,
		}
		s.Users[key] = user
	}
	if discordID != "" {
		user.DiscordID = discordID
	}
	if discordName != "" {
		user.DiscordName = discordName
	}
	if wplaceID != "" {
		user.WplaceID = wplaceID
	}
	if wplaceName != "" {
		user.WplaceName = wplaceName
	}
}

func resolveIdentityKey(discordID, wplaceID string) string {
	if discordID != "" {
		return discordID
	}
	if wplaceID != "" {
		return wplaceIdentityKey(wplaceID)
	}
	return ""
}

func wplaceIdentityKey(wplaceID string) string {
	return fmt.Sprintf("wplace:%s", wplaceID)
}
