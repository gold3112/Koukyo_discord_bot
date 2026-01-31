package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"Koukyo_discord_bot/internal/activity"
	"Koukyo_discord_bot/internal/utils"

	"github.com/bwmarrin/discordgo"
)

const (
	meLinkTileX      = 1755
	meLinkTileY      = 55
	meLinkPixelMargin = 2
	meLinkMaxSessions = 20
	meLinkPickAttempts = 50
	meLinkTimeout    = 1 * time.Minute
	meLinkPollEvery  = 10 * time.Second
	meLinkZoom       = 21.17
)

type meLinkSession struct {
	userID    string
	pixelX    int
	pixelY    int
	initialID int
	startedAt time.Time
	notify   func(string)
}

type meLinkManager struct {
	mu       sync.Mutex
	byUser   map[string]*meLinkSession
	inUsePos map[string]string
}

var globalMeLinkManager = &meLinkManager{
	byUser:   make(map[string]*meLinkSession),
	inUsePos: make(map[string]string),
}

var meLinkRng = rand.New(rand.NewSource(time.Now().UnixNano()))

func (m *meLinkManager) acquire(userID string) (*meLinkSession, time.Duration, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if existing := m.byUser[userID]; existing != nil {
		remaining := meLinkTimeout - time.Since(existing.startedAt)
		if remaining < 0 {
			remaining = 0
		}
		return existing, remaining, false
	}

	if len(m.byUser) >= meLinkMaxSessions {
		return nil, 0, false
	}

	now := time.Now()
	minPix := meLinkPixelMargin
	maxPix := utils.WplaceTileSize - 1 - meLinkPixelMargin
	if minPix < 0 {
		minPix = 0
	}
	if maxPix < minPix {
		maxPix = minPix
	}

	for i := 0; i < meLinkPickAttempts; i++ {
		px := minPix + meLinkRng.Intn(maxPix-minPix+1)
		py := minPix + meLinkRng.Intn(maxPix-minPix+1)
		key := pixelKeyForLink(px, py)
		if _, used := m.inUsePos[key]; used {
			continue
		}
		session := &meLinkSession{
			userID:    userID,
			pixelX:    px,
			pixelY:    py,
			initialID: 0,
			startedAt: now,
		}
		m.byUser[userID] = session
		m.inUsePos[key] = userID
		return session, meLinkTimeout, true
	}
	return nil, 0, false
}

func (m *meLinkManager) release(userID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	session := m.byUser[userID]
	if session == nil {
		return
	}
	delete(m.inUsePos, pixelKeyForLink(session.pixelX, session.pixelY))
	delete(m.byUser, userID)
}

func (c *MeCommand) startLinkFlow(
	s *discordgo.Session,
	user *discordgo.User,
	sendFallback func(content string) error,
) error {
	session, remaining, created := globalMeLinkManager.acquire(user.ID)
	if !created {
		if session == nil {
			return sendFallback("⏳ 連携が混雑しています。少し時間をおいて再試行してください。")
		}
		return sendFallback(fmt.Sprintf("⏳ 既に紐づけ処理中です。残り時間: %s\n座標: (%d, %d) (タイル %d-%d)",
			remaining.Round(time.Second), session.pixelX, session.pixelY, meLinkTileX, meLinkTileY))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	resp, status, err := activity.FetchPixelInfo(ctx, c.httpClient, c.limiter, meLinkTileX, meLinkTileY, session.pixelX, session.pixelY)
	if err != nil {
		globalMeLinkManager.release(user.ID)
		return sendFallback(fmt.Sprintf("❌ 初期チェックに失敗しました: %v (status=%d)", err, status))
	}
	if resp != nil && resp.PaintedBy != nil {
		session.initialID = resp.PaintedBy.ID
	}

	latLng := utils.TilePixelCenterToLngLat(meLinkTileX, meLinkTileY, session.pixelX, session.pixelY)
	url := utils.BuildWplaceURL(latLng.Lng, latLng.Lat, meLinkZoom)
	instruction := strings.TrimSpace(fmt.Sprintf(
		"✅ Wplace連携の確認を開始します。\n"+
			"1分以内にこのURLを開き、指定ピクセルに色を置いてください。\n"+
			"※ 既に塗ったことがある場合は「別の色で塗り直し」してください。\n"+
			"URL: %s\n"+
			"タイル: %d-%d / ピクセル: (%d, %d)",
		url, meLinkTileX, meLinkTileY, session.pixelX, session.pixelY,
	))

	session.notify = func(msg string) {
		_ = sendFallback(msg)
	}

	if err := sendDM(s, user.ID, instruction); err != nil {
		_ = sendFallback("⚠️ DMに送信できませんでした。以下のURLを使用してください。\n" + instruction)
	} else {
		_ = sendFallback("📩 DMに認証用URLを送信しました。進捗はこのチャンネルにも通知します。")
	}

	go c.pollLinkResult(s, user, session)
	return nil
}

func (c *MeCommand) pollLinkResult(s *discordgo.Session, user *discordgo.User, session *meLinkSession) {
	defer globalMeLinkManager.release(user.ID)

	ctx, cancel := context.WithTimeout(context.Background(), meLinkTimeout)
	defer cancel()

	ticker := time.NewTicker(meLinkPollEvery)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			timeoutMsg := "⏱️ 認証時間が経過しました。/me を再実行してください。"
			_ = sendDM(s, user.ID, timeoutMsg)
			if session.notify != nil {
				session.notify(timeoutMsg)
			}
			return
		case <-ticker.C:
			checkCtx, cancelCheck := context.WithTimeout(context.Background(), 8*time.Second)
			resp, status, err := activity.FetchPixelInfo(checkCtx, c.httpClient, c.limiter, meLinkTileX, meLinkTileY, session.pixelX, session.pixelY)
			cancelCheck()
			if err != nil {
				if status == httpStatusTooManyRequests {
					continue
				}
				continue
			}
			if resp == nil || resp.PaintedBy == nil {
				continue
			}
			if resp.PaintedBy.ID == session.initialID {
				continue
			}
			entry, err := updateUserActivityLink(c.dataDir, resp.PaintedBy, user)
			if err != nil {
				errMsg := fmt.Sprintf("❌ 紐づけに失敗しました: %v", err)
				_ = sendDM(s, user.ID, errMsg)
				if session.notify != nil {
					session.notify(errMsg)
				}
				return
			}
			embed, file := buildMeCardEmbed(entry, user)
			_ = sendDMEmbed(s, user.ID, embed, file)
			if session.notify != nil {
				session.notify("✅ 連携が完了しました。DMにユーザーカードを送信しました。")
			}
			return
		}
	}
}

func updateUserActivityLink(dataDir string, painter *activity.PaintedBy, user *discordgo.User) (userActivityEntry, error) {
	path := filepath.Join(dataDir, "user_activity.json")
	raw := make(map[string]*activity.UserActivity)
	if data, err := os.ReadFile(path); err == nil && len(data) > 0 {
		_ = json.Unmarshal(data, &raw)
	}
	painterID := strconv.Itoa(painter.ID)
	entry := raw[painterID]
	if entry == nil {
		entry = &activity.UserActivity{
			ID: painterID,
		}
	}
	if entry.DiscordID != "" && entry.DiscordID != user.ID {
		return userActivityEntry{}, fmt.Errorf("このWplaceユーザーは別のDiscordと紐づけ済みです")
	}
	if painter.Name != "" {
		entry.Name = painter.Name
	}
	if painter.AllianceName != "" {
		entry.AllianceName = painter.AllianceName
	}
	if painter.Picture != "" {
		entry.Picture = painter.Picture
	}
	entry.DiscordID = user.ID
	entry.Discord = discordTag(user)
	entry.LastSeen = time.Now().UTC().Format(time.RFC3339Nano)
	raw[painterID] = entry

	payload, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return userActivityEntry{}, err
	}
	if err := os.WriteFile(path, payload, 0644); err != nil {
		return userActivityEntry{}, err
	}
	return userActivityEntry{
		ID:            painterID,
		Name:          entry.Name,
		Alliance:      entry.AllianceName,
		Discord:       entry.Discord,
		DiscordID:     entry.DiscordID,
		Picture:       entry.Picture,
		VandalCount:   entry.VandalCount,
		RestoredCount: entry.RestoredCount,
		Score:         entry.ActivityScore,
		LastSeen:      parseUserListTime(entry.LastSeen),
	}, nil
}

func sendDM(s *discordgo.Session, userID, content string) error {
	channel, err := s.UserChannelCreate(userID)
	if err != nil {
		return err
	}
	_, err = s.ChannelMessageSend(channel.ID, content)
	return err
}

func sendDMEmbed(s *discordgo.Session, userID string, embed *discordgo.MessageEmbed, file *discordgo.File) error {
	channel, err := s.UserChannelCreate(userID)
	if err != nil {
		return err
	}
	_, err = s.ChannelMessageSendComplex(channel.ID, &discordgo.MessageSend{
		Embeds: []*discordgo.MessageEmbed{embed},
		Files:  buildOptionalFiles(file),
	})
	return err
}

func pixelKeyForLink(x, y int) string {
	return fmt.Sprintf("%d,%d", x, y)
}

const httpStatusTooManyRequests = 429
