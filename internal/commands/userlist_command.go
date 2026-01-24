package commands

import (
	"Koukyo_discord_bot/internal/activity"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

const (
	userListKindFix = "fix"
	userListKindGrf = "grf"

	userListModeRanking = "ranking"
	userListModeRecent  = "recent"

	userListPageSize = 10
	userListPrefix   = "userlist:"
)

type UserListCommand struct {
	kind    string
	dataDir string
}

func NewFixUserCommand(dataDir string) *UserListCommand {
	return &UserListCommand{kind: userListKindFix, dataDir: dataDir}
}

func NewGrfUserCommand(dataDir string) *UserListCommand {
	return &UserListCommand{kind: userListKindGrf, dataDir: dataDir}
}

func (c *UserListCommand) Name() string {
	if c.kind == userListKindFix {
		return "fixuser"
	}
	return "grfuser"
}

func (c *UserListCommand) Description() string {
	if c.kind == userListKindFix {
		return "修復ユーザーの一覧を表示します"
	}
	return "荒らしユーザーの一覧を表示します"
}

func (c *UserListCommand) ExecuteText(s *discordgo.Session, m *discordgo.MessageCreate, args []string) error {
	mode := userListModeRanking
	if len(args) > 0 {
		mode = normalizeUserListMode(args[0])
	}
	page := 0
	return sendUserListMessage(s, m.ChannelID, c.dataDir, c.kind, mode, page)
}

func (c *UserListCommand) ExecuteSlash(s *discordgo.Session, i *discordgo.InteractionCreate) error {
	mode := userListModeRanking
	page := 0
	for _, opt := range i.ApplicationCommandData().Options {
		switch opt.Name {
		case "mode":
			mode = normalizeUserListMode(opt.StringValue())
		case "page":
			page = int(opt.IntValue()) - 1
		}
	}
	if page < 0 {
		page = 0
	}

	embed, components, err := buildUserListEmbed(c.dataDir, c.kind, mode, page)
	if err != nil {
		return respondUserListError(s, i, err)
	}

	return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds:     []*discordgo.MessageEmbed{embed},
			Components: components,
		},
	})
}

func (c *UserListCommand) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "mode",
				Description: "表示方式 (ranking / recent)",
				Required:    false,
				Choices: []*discordgo.ApplicationCommandOptionChoice{
					{Name: "ランキング", Value: userListModeRanking},
					{Name: "最終観測日", Value: userListModeRecent},
				},
			},
			{
				Type:        discordgo.ApplicationCommandOptionInteger,
				Name:        "page",
				Description: "ページ番号 (1から)",
				Required:    false,
				MinValue:    func() *float64 { v := 1.0; return &v }(),
			},
		},
	}
}

func HandleUserListPagination(s *discordgo.Session, i *discordgo.InteractionCreate, dataDir string) {
	customID := i.MessageComponentData().CustomID
	if !strings.HasPrefix(customID, userListPrefix) {
		return
	}
	parts := strings.Split(customID, ":")
	if len(parts) != 4 {
		return
	}
	kind := parts[1]
	mode := normalizeUserListMode(parts[2])
	page, err := strconv.Atoi(parts[3])
	if err != nil {
		page = 0
	}
	if page < 0 {
		page = 0
	}

	embed, components, err := buildUserListEmbed(dataDir, kind, mode, page)
	if err != nil {
		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "❌ エラー: " + err.Error(),
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Embeds:     []*discordgo.MessageEmbed{embed},
			Components: components,
		},
	})
}

type userListEntry struct {
	ID       string
	Name     string
	Alliance string
	Count    int
	LastSeen time.Time
}

func buildUserListEmbed(dataDir, kind, mode string, page int) (*discordgo.MessageEmbed, []discordgo.MessageComponent, error) {
	entries, err := loadUserListEntries(dataDir, kind)
	if err != nil {
		return nil, nil, err
	}
	if mode == userListModeRecent {
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].LastSeen.After(entries[j].LastSeen)
		})
	} else {
		sort.Slice(entries, func(i, j int) bool {
			if entries[i].Count == entries[j].Count {
				return entries[i].Name < entries[j].Name
			}
			return entries[i].Count > entries[j].Count
		})
	}

	title := "🚨 荒らしユーザー一覧"
	if kind == userListKindFix {
		title = "🛠️ 修復ユーザー一覧"
	}

	total := len(entries)
	maxPage := 0
	if total > 0 {
		maxPage = (total - 1) / userListPageSize
	}
	if page > maxPage {
		page = maxPage
	}

	start := page * userListPageSize
	end := start + userListPageSize
	if end > total {
		end = total
	}

	lines := make([]string, 0)
	jst := time.FixedZone("JST", 9*3600)
	for i := start; i < end; i++ {
		entry := entries[i]
		name := entry.Name
		if name == "" {
			name = fmt.Sprintf("ID:%s", entry.ID)
		}
		if entry.Alliance != "" {
			name = fmt.Sprintf("%s (%s)", name, entry.Alliance)
		}
		lastSeenText := "-"
		if !entry.LastSeen.IsZero() {
			lastSeenText = entry.LastSeen.In(jst).Format("2006-01-02 15:04")
		}
		lines = append(lines, fmt.Sprintf("%d. %s | %d | 最終 %s", i+1, name, entry.Count, lastSeenText))
	}
	if len(lines) == 0 {
		lines = append(lines, "該当なし")
	}

	modeLabel := "ランキング"
	if mode == userListModeRecent {
		modeLabel = "最終観測日"
	}
	countLabel := "荒らし数"
	if kind == userListKindFix {
		countLabel = "修復数"
	}
	description := fmt.Sprintf("表示方式: %s | %s: %d件", modeLabel, countLabel, total)

	embed := &discordgo.MessageEmbed{
		Title:       title,
		Description: description,
		Color:       0x3498DB,
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:  fmt.Sprintf("ページ %d / %d", page+1, maxPage+1),
				Value: strings.Join(lines, "\n"),
			},
		},
		Footer: &discordgo.MessageEmbedFooter{
			Text: fmt.Sprintf("%s | %s", countLabel, modeLabel),
		},
		Timestamp: time.Now().Format(time.RFC3339),
	}

	components := []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{
					Label:    "前へ",
					Style:    discordgo.PrimaryButton,
					CustomID: fmt.Sprintf("%s%s:%s:%d", userListPrefix, kind, mode, page-1),
					Disabled: page <= 0,
				},
				discordgo.Button{
					Label:    "次へ",
					Style:    discordgo.PrimaryButton,
					CustomID: fmt.Sprintf("%s%s:%s:%d", userListPrefix, kind, mode, page+1),
					Disabled: page >= maxPage,
				},
			},
		},
	}
	return embed, components, nil
}

func loadUserListEntries(dataDir, kind string) ([]userListEntry, error) {
	path := filepath.Join(dataDir, "user_activity.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var raw map[string]*activity.UserActivity
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	entries := make([]userListEntry, 0, len(raw))
	for id, entry := range raw {
		count := entry.VandalCount
		if kind == userListKindFix {
			count = entry.RestoredCount
		}
		lastSeen := parseUserListTime(entry.LastSeen)
		entries = append(entries, userListEntry{
			ID:       id,
			Name:     entry.Name,
			Alliance: entry.AllianceName,
			Count:    count,
			LastSeen: lastSeen,
		})
	}
	return entries, nil
}

func parseUserListTime(value string) time.Time {
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

func normalizeUserListMode(mode string) string {
	switch strings.ToLower(mode) {
	case userListModeRecent, "last", "latest":
		return userListModeRecent
	default:
		return userListModeRanking
	}
}

func respondUserListError(s *discordgo.Session, i *discordgo.InteractionCreate, err error) error {
	return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: "❌ エラー: " + err.Error(),
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
}

func sendUserListMessage(s *discordgo.Session, channelID, dataDir, kind, mode string, page int) error {
	embed, components, err := buildUserListEmbed(dataDir, kind, mode, page)
	if err != nil {
		_, e := s.ChannelMessageSend(channelID, "❌ エラー: "+err.Error())
		return e
	}
	_, err = s.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
		Embeds:     []*discordgo.MessageEmbed{embed},
		Components: components,
	})
	return err
}
