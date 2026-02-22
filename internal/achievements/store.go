package achievements

import (
	"Koukyo_discord_bot/internal/utils"
	"encoding/json"
	"fmt"
	"os"
	"strings"
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
	user, _ := s.findUserByDiscordID(discordID)
	return user
}

func (s *Store) GetByWplaceID(wplaceID string) *UserAchievements {
	user, _ := s.findUserByWplaceID(wplaceID)
	return user
}

func (s *Store) GetByIdentity(discordID, wplaceID string) *UserAchievements {
	if user, _ := s.findUserByDiscordID(discordID); user != nil {
		return user
	}
	if user, _ := s.findUserByWplaceID(wplaceID); user != nil {
		return user
	}
	return nil
}

func (s *Store) Award(discordID string, achievement Achievement) bool {
	return s.AwardByIdentity(discordID, "", achievement)
}

func (s *Store) AwardByIdentity(discordID, wplaceID string, achievement Achievement) bool {
	if s == nil || achievement.ID == "" {
		return false
	}
	user := s.ensureIdentityUser(discordID, wplaceID)
	if user == nil {
		return false
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
	if s == nil {
		return
	}
	user := s.ensureIdentityUser(discordID, wplaceID)
	if user == nil {
		return
	}
	discordName = strings.TrimSpace(discordName)
	wplaceName = strings.TrimSpace(wplaceName)
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

func (s *Store) findUserByDiscordID(discordID string) (*UserAchievements, string) {
	if s == nil || s.Users == nil {
		return nil, ""
	}
	discordID = strings.TrimSpace(discordID)
	if discordID == "" {
		return nil, ""
	}
	if user, ok := s.Users[discordID]; ok && user != nil {
		return user, discordID
	}
	for key, user := range s.Users {
		if user == nil {
			continue
		}
		if strings.TrimSpace(user.DiscordID) == discordID {
			return user, key
		}
	}
	return nil, ""
}

func (s *Store) findUserByWplaceID(wplaceID string) (*UserAchievements, string) {
	if s == nil || s.Users == nil {
		return nil, ""
	}
	wplaceID = strings.TrimSpace(wplaceID)
	if wplaceID == "" {
		return nil, ""
	}
	key := wplaceIdentityKey(wplaceID)
	if user, ok := s.Users[key]; ok && user != nil {
		return user, key
	}
	for mapKey, user := range s.Users {
		if user == nil {
			continue
		}
		if strings.TrimSpace(user.WplaceID) == wplaceID {
			return user, mapKey
		}
	}
	return nil, ""
}

func (s *Store) ensureIdentityUser(discordID, wplaceID string) *UserAchievements {
	if s == nil {
		return nil
	}
	discordID = strings.TrimSpace(discordID)
	wplaceID = strings.TrimSpace(wplaceID)
	key := resolveIdentityKey(discordID, wplaceID)
	if key == "" {
		return nil
	}
	if s.Users == nil {
		s.Users = map[string]*UserAchievements{}
	}

	discordUser, discordKey := s.findUserByDiscordID(discordID)
	wplaceUser, wplaceKey := s.findUserByWplaceID(wplaceID)

	user := discordUser
	userKey := discordKey
	if user == nil {
		user = wplaceUser
		userKey = wplaceKey
	}
	if user == nil {
		user = &UserAchievements{}
	}

	if discordUser != nil && wplaceUser != nil && discordUser != wplaceUser {
		mergeUserRecords(discordUser, wplaceUser)
		user = discordUser
		userKey = discordKey
		if wplaceKey != "" {
			delete(s.Users, wplaceKey)
		}
	}

	if userKey != "" && userKey != key {
		delete(s.Users, userKey)
	}
	if discordID != "" && wplaceKey != "" && wplaceKey != key {
		delete(s.Users, wplaceKey)
	}
	s.Users[key] = user

	if discordID != "" {
		user.DiscordID = discordID
	}
	if wplaceID != "" {
		user.WplaceID = wplaceID
	}
	return user
}

func mergeUserRecords(dst, src *UserAchievements) {
	if dst == nil || src == nil || dst == src {
		return
	}
	if dst.DiscordID == "" {
		dst.DiscordID = src.DiscordID
	}
	if dst.DiscordName == "" {
		dst.DiscordName = src.DiscordName
	}
	if dst.WplaceID == "" {
		dst.WplaceID = src.WplaceID
	}
	if dst.WplaceName == "" {
		dst.WplaceName = src.WplaceName
	}
	if len(src.Achievements) == 0 {
		return
	}

	indexByID := map[string]int{}
	for i, a := range dst.Achievements {
		if a.ID == "" {
			continue
		}
		indexByID[a.ID] = i
	}
	for _, a := range src.Achievements {
		if a.ID == "" {
			continue
		}
		if idx, ok := indexByID[a.ID]; ok {
			dst.Achievements[idx] = mergeAchievement(dst.Achievements[idx], a)
			continue
		}
		dst.Achievements = append(dst.Achievements, a)
		indexByID[a.ID] = len(dst.Achievements) - 1
	}
}

func mergeAchievement(dst, src Achievement) Achievement {
	if dst.Name == "" {
		dst.Name = src.Name
	}
	if dst.Description == "" {
		dst.Description = src.Description
	}
	dst.AwardedAt = earlierAwardedAt(dst.AwardedAt, src.AwardedAt)
	return dst
}

func earlierAwardedAt(a, b string) string {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == "" {
		return b
	}
	if b == "" {
		return a
	}
	ta, errA := parseAwardedAt(a)
	tb, errB := parseAwardedAt(b)
	if errA == nil && errB == nil {
		if tb.Before(ta) {
			return b
		}
		return a
	}
	if errA != nil && errB == nil {
		return b
	}
	return a
}

func parseAwardedAt(value string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return t, nil
	}
	return time.Parse(time.RFC3339, value)
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
