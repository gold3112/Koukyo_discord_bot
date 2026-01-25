package notifications

import (
	"Koukyo_discord_bot/internal/activity"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

func (n *Notifier) startDailyRankingLoop() {
	go func() {
		jst := time.FixedZone("JST", 9*3600)
		for {
			now := time.Now().In(jst)
			next := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, jst)
			sleep := time.Until(next)
			if sleep < time.Second {
				sleep = time.Second
			}
			timer := time.NewTimer(sleep)
			<-timer.C

			reportTime := next.Add(-time.Second).In(jst)
			reportDate := reportTime.Format("2006-01-02")
			if reportDate == n.lastDailyReportDate {
				continue
			}
			if err := n.sendDailyRankingReport(reportTime); err != nil {
				log.Printf("Failed to send daily ranking report: %v", err)
				continue
			}
			n.lastDailyReportDate = reportDate
		}
	}()
}

type rankingEntry struct {
	ID       string
	Name     string
	Alliance string
	Count    int
}

func (n *Notifier) sendDailyRankingReport(reportTime time.Time) error {
	if n.dataDir == "" {
		return fmt.Errorf("dataDir is empty")
	}
	path := filepath.Join(n.dataDir, "user_activity.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var entries map[string]*activity.UserActivity
	if err := json.Unmarshal(data, &entries); err != nil {
		return err
	}

	jst := time.FixedZone("JST", 9*3600)
	dateKey := reportTime.In(jst).Format("2006-01-02")

	vandals := buildRanking(entries, dateKey, true)
	restores := buildRanking(entries, dateKey, false)

	vandalText := formatRanking(vandals)
	restoreText := formatRanking(restores)

	titleDate := reportTime.In(jst).Format("2006-01-02 (JST)")
	embed := &discordgo.MessageEmbed{
		Title:       "📊 日次ランキング",
		Description: fmt.Sprintf("%s の荒らし/修復ランキング", titleDate),
		Color:       0x1E90FF,
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:   "🚨 荒らしランキング",
				Value:  vandalText,
				Inline: false,
			},
			{
				Name:   "🛠️ 修復ランキング",
				Value:  restoreText,
				Inline: false,
			},
		},
		Timestamp: time.Now().Format(time.RFC3339),
		Footer: &discordgo.MessageEmbedFooter{
			Text: "自動日次レポート",
		},
	}

	for _, guild := range n.session.State.Guilds {
		gs := n.settings.GetGuildSettings(guild.ID)
		if !gs.AutoNotifyEnabled || gs.NotificationChannel == nil {
			continue
		}
		_, err := n.session.ChannelMessageSendComplex(*gs.NotificationChannel, &discordgo.MessageSend{
			Embeds: []*discordgo.MessageEmbed{embed},
		})
		if err != nil {
			log.Printf("Failed to send daily ranking to guild %s: %v", guild.ID, err)
		} else {
			log.Printf("Sent daily ranking to guild %s", guild.ID)
		}
	}

	return nil
}

func buildRanking(entries map[string]*activity.UserActivity, dateKey string, vandal bool) []rankingEntry {
	out := make([]rankingEntry, 0)
	for id, entry := range entries {
		var count int
		if vandal {
			count = entry.DailyVandalCounts[dateKey]
		} else {
			count = entry.DailyRestoredCounts[dateKey]
		}
		if count <= 0 {
			continue
		}
		name := entry.Name
		if name == "" {
			name = fmt.Sprintf("ID:%s", id)
		}
		out = append(out, rankingEntry{
			ID:       id,
			Name:     name,
			Alliance: entry.AllianceName,
			Count:    count,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count == out[j].Count {
			return out[i].Name < out[j].Name
		}
		return out[i].Count > out[j].Count
	})
	return out
}

func formatRanking(entries []rankingEntry) string {
	if len(entries) == 0 {
		return "該当なし"
	}
	limit := 10
	if len(entries) < limit {
		limit = len(entries)
	}
	lines := make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		entry := entries[i]
		display := entry.Name
		if entry.Alliance != "" {
			display = fmt.Sprintf("%s (%s)", display, entry.Alliance)
		}
		lines = append(lines, fmt.Sprintf("%d. %s - %d", i+1, display, entry.Count))
	}
	return strings.Join(lines, "\n")
}
