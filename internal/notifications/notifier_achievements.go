package notifications

import (
	"Koukyo_discord_bot/internal/achievements"
	"Koukyo_discord_bot/internal/activity"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const achievementEvalInterval = 1 * time.Minute

type achievementNotice struct {
	DiscordID       string
	DiscordName     string
	WplaceID        string
	WplaceName      string
	AchievementName string
}

func (n *Notifier) startAchievementLoop() {
	go func() {
		// Start a few seconds after boot to let state caches warm up.
		time.Sleep(8 * time.Second)
		n.evaluateAchievementsAndNotify()

		ticker := time.NewTicker(achievementEvalInterval)
		defer ticker.Stop()
		for range ticker.C {
			n.evaluateAchievementsAndNotify()
		}
	}()
}

func (n *Notifier) evaluateAchievementsAndNotify() {
	if n == nil || n.settings == nil || n.session == nil {
		return
	}
	if n.dataDir == "" {
		return
	}

	n.achievementEvalMu.Lock()
	defer n.achievementEvalMu.Unlock()

	rulePath := filepath.Join(n.dataDir, "achievement_rules.json")
	if err := achievements.EnsureRuleSetFile(rulePath); err != nil {
		log.Printf("achievement eval: failed to ensure rule file: %v", err)
		return
	}
	ruleSet, err := achievements.LoadRuleSet(rulePath)
	if err != nil {
		log.Printf("achievement eval: failed to load rules: %v", err)
		return
	}

	activityPath := filepath.Join(n.dataDir, "user_activity.json")
	entries, err := readUserActivityWithRetry(activityPath, 3, 100*time.Millisecond)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return
		}
		log.Printf("achievement eval: failed to load user activity: %v", err)
		return
	}

	storePath := filepath.Join(n.dataDir, "achievements.json")
	store, err := achievements.Load(storePath)
	if err != nil {
		log.Printf("achievement eval: failed to load store: %v", err)
		return
	}

	awardedCount := 0
	pendingNotices := make([]achievementNotice, 0)

	for wplaceID, entry := range entries {
		if entry == nil {
			continue
		}
		wplaceID = strings.TrimSpace(wplaceID)
		if wplaceID == "" {
			continue
		}
		discordID := strings.TrimSpace(entry.DiscordID)

		store.UpsertUserProfile(discordID, strings.TrimSpace(entry.Discord), wplaceID, strings.TrimSpace(entry.Name))

		snapshot := activityToSnapshot(wplaceID, entry)
		newAwards := achievements.Evaluate(snapshot, ruleSet)
		for _, award := range newAwards {
			if !store.AwardByIdentity(discordID, wplaceID, award) {
				continue
			}
			awardedCount++
			pendingNotices = append(pendingNotices, achievementNotice{
				DiscordID:       discordID,
				DiscordName:     strings.TrimSpace(entry.Discord),
				WplaceID:        wplaceID,
				WplaceName:      strings.TrimSpace(entry.Name),
				AchievementName: award.Name,
			})
		}
	}

	initialSync := !n.achievementBaselineReady
	if awardedCount > 0 {
		if err := achievements.Save(storePath, store); err != nil {
			log.Printf("achievement eval: failed to save store: %v", err)
			return
		}
	}

	if initialSync {
		n.achievementBaselineReady = true
		if awardedCount > 0 {
			log.Printf("achievement eval: baseline sync completed, %d achievements saved (notifications suppressed)", awardedCount)
		}
		return
	}
	if awardedCount == 0 {
		return
	}

	for _, notice := range pendingNotices {
		userDisplay := buildAchievementUserDisplay(notice)
		for _, guild := range n.session.State.Guilds {
			n.NotifyAchievement(guild.ID, userDisplay, notice.AchievementName)
		}
	}

	log.Printf("achievement eval: awarded %d achievements", awardedCount)
}

func activityToSnapshot(wplaceID string, entry *activity.UserActivity) achievements.UserSnapshot {
	snapshot := achievements.UserSnapshot{
		DiscordID:          strings.TrimSpace(entry.DiscordID),
		DiscordName:        strings.TrimSpace(entry.Discord),
		WplaceID:           strings.TrimSpace(wplaceID),
		WplaceName:         strings.TrimSpace(entry.Name),
		VandalCount:        entry.VandalCount,
		RestoredCount:      entry.RestoredCount,
		ActivityScore:      entry.ActivityScore,
		LastSeenAt:         parseActivityLastSeen(entry.LastSeen),
		DailyVandalCounts:  map[string]int{},
		DailyRestoreCounts: map[string]int{},
	}
	for day, count := range entry.DailyVandalCounts {
		snapshot.DailyVandalCounts[day] = count
	}
	for day, count := range entry.DailyRestoredCounts {
		snapshot.DailyRestoreCounts[day] = count
	}
	return snapshot
}

func parseActivityLastSeen(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return t
	}
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t
	}
	return time.Time{}
}

func buildAchievementUserDisplay(notice achievementNotice) string {
	if notice.WplaceName != "" {
		return notice.WplaceName
	}
	if notice.WplaceID != "" {
		return "ID:" + notice.WplaceID
	}
	if notice.DiscordName != "" {
		return notice.DiscordName
	}
	if notice.DiscordID != "" {
		return fmt.Sprintf("Discord:%s", notice.DiscordID)
	}
	return "unknown"
}
