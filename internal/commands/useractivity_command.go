package commands

import (
	"Koukyo_discord_bot/internal/activity"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

const (
	userActivityPrefix         = "useractivity:"
	userActivitySelectPrefix   = "useractivity_select"
	userActivityModeDetail     = "detail"
	userActivityMaxSelectItems = 25
	userActivityIconSize       = 160
	userActivityColorsNb       = 9
	userActivitySaturation     = 95
	userActivityLightness      = 45
	userActivityMagicNumber    = 5
)

type UserActivityCommand struct {
	dataDir string
}

func NewUserActivityCommand(dataDir string) *UserActivityCommand {
	return &UserActivityCommand{dataDir: dataDir}
}

func (c *UserActivityCommand) Name() string { return "useractivity" }
func (c *UserActivityCommand) Description() string {
	return "ユーザー活動を確認します (ID検索/詳細表示に対応)"
}

func (c *UserActivityCommand) ExecuteText(s *discordgo.Session, m *discordgo.MessageCreate, args []string) error {
	_, err := s.ChannelMessageSend(m.ChannelID, "このコマンドはスラッシュコマンドで利用してください。")
	return err
}

func (c *UserActivityCommand) ExecuteSlash(s *discordgo.Session, i *discordgo.InteractionCreate) error {
	kind := userListKindGrf
	mode := userListModeRanking
	listType := userListTypeScore
	page := 0
	userID := ""
	discordID := ""
	discordName := ""
	for _, opt := range i.ApplicationCommandData().Options {
		switch opt.Name {
		case "id":
			userID = strings.TrimSpace(opt.StringValue())
		case "discord_id":
			discordID = strings.TrimSpace(opt.StringValue())
		case "discord":
			discordName = strings.TrimSpace(opt.StringValue())
		case "kind":
			kind = normalizeUserListKind(opt.StringValue())
		case "mode":
			mode = normalizeUserListMode(opt.StringValue())
			if opt.StringValue() == userActivityModeDetail {
				mode = userActivityModeDetail
			}
		case "type":
			listType = normalizeUserListType(opt.StringValue())
		case "page":
			page = int(opt.IntValue()) - 1
		}
	}
	if page < 0 {
		page = 0
	}

	if userID != "" || discordID != "" {
		entry, err := loadUserActivityByID(c.dataDir, userID, discordID)
		if err != nil {
			return respondUserListError(s, i, err)
		}
		embed, file := buildUserActivityDetailEmbedFromEntry(kind, entry)
		return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Embeds: []*discordgo.MessageEmbed{embed},
				Files:  buildOptionalFiles(file),
			},
		})
	}

	if discordName != "" {
		matches, err := loadUserActivityByDiscordName(c.dataDir, discordName)
		if err != nil {
			return respondUserListError(s, i, err)
		}
		if len(matches) == 0 {
			return respondUserListError(s, i, fmt.Errorf("該当ユーザーが見つかりません"))
		}
		if len(matches) == 1 {
			embed, file := buildUserActivityDetailEmbedFromEntry(kind, matches[0])
			return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Embeds: []*discordgo.MessageEmbed{embed},
					Files:  buildOptionalFiles(file),
				},
			})
		}
		embed, components := buildUserActivitySearchEmbed(discordName, matches)
		return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Embeds:     []*discordgo.MessageEmbed{embed},
				Components: components,
			},
		})
	}

	if mode == userActivityModeDetail {
		embed, components, file, err := buildUserActivityDetailEmbed(c.dataDir, kind, listType, page)
		if err != nil {
			return respondUserListError(s, i, err)
		}
		return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Embeds:     []*discordgo.MessageEmbed{embed},
				Components: components,
				Files:      buildOptionalFiles(file),
			},
		})
	}

	embed, components, err := buildUserListEmbed(c.dataDir, kind, mode, listType, page)
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

func (c *UserActivityCommand) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "id",
				Description: "ゲーム内ID",
				Required:    false,
			},
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "discord",
				Description: "Discord名 (部分一致)",
				Required:    false,
			},
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "discord_id",
				Description: "Discord ID",
				Required:    false,
			},
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "kind",
				Description: "対象: vandal | fix",
				Required:    false,
				Choices: []*discordgo.ApplicationCommandOptionChoice{
					{Name: "vandal", Value: userListKindGrf},
					{Name: "fix", Value: userListKindFix},
				},
			},
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "mode",
				Description: "表示方式 (ranking / recent / detail)",
				Required:    false,
				Choices: []*discordgo.ApplicationCommandOptionChoice{
					{Name: "ランキング", Value: userListModeRanking},
					{Name: "最終観測日", Value: userListModeRecent},
					{Name: "詳細", Value: userActivityModeDetail},
				},
			},
			{
				Type:        discordgo.ApplicationCommandOptionInteger,
				Name:        "page",
				Description: "ページ番号 (1から)",
				Required:    false,
				MinValue:    func() *float64 { v := 1.0; return &v }(),
			},
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "type",
				Description: "集計方法: score | absolute",
				Required:    false,
				Choices: []*discordgo.ApplicationCommandOptionChoice{
					{Name: "score", Value: userListTypeScore},
					{Name: "absolute", Value: userListTypeAbsolute},
				},
			},
		},
	}
}

func HandleUserActivityPagination(s *discordgo.Session, i *discordgo.InteractionCreate, dataDir string) {
	customID := i.MessageComponentData().CustomID
	if !strings.HasPrefix(customID, userActivityPrefix) {
		return
	}
	parts := strings.Split(customID, ":")
	if len(parts) != 4 {
		return
	}
	kind := normalizeUserListKind(parts[1])
	listType := normalizeUserListType(parts[2])
	page, err := strconv.Atoi(parts[3])
	if err != nil {
		page = 0
	}
	if page < 0 {
		page = 0
	}

	embed, components, file, err := buildUserActivityDetailEmbed(dataDir, kind, listType, page)
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
			Files:      buildOptionalFiles(file),
		},
	})
}

func HandleUserActivitySelect(s *discordgo.Session, i *discordgo.InteractionCreate, dataDir string) {
	customID := i.MessageComponentData().CustomID
	if customID != userActivitySelectPrefix {
		return
	}
	values := i.MessageComponentData().Values
	if len(values) == 0 {
		return
	}
	entry, err := loadUserActivityByID(dataDir, values[0], "")
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
	embed, file := buildUserActivityDetailEmbedFromEntry(userListKindGrf, entry)
	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
			Files:  buildOptionalFiles(file),
		},
	})
}

type userActivityEntry struct {
	ID            string
	Name          string
	Alliance      string
	Discord       string
	DiscordID     string
	Picture       string
	VandalCount   int
	RestoredCount int
	Score         int
	LastSeen      time.Time
}

func buildUserActivityDetailEmbed(dataDir, kind, listType string, page int) (*discordgo.MessageEmbed, []discordgo.MessageComponent, *discordgo.File, error) {
	entries, err := loadUserActivityEntries(dataDir, kind, listType)
	if err != nil {
		return nil, nil, nil, err
	}
	if len(entries) == 0 {
		return nil, nil, nil, fmt.Errorf("該当ユーザーが見つかりません")
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Score == entries[j].Score {
			return entries[i].Name < entries[j].Name
		}
		return entries[i].Score > entries[j].Score
	})
	if page < 0 {
		page = 0
	}
	if page >= len(entries) {
		page = len(entries) - 1
	}
	entry := entries[page]
	embed, file := buildUserActivityDetailEmbedFromEntry(kind, entry)
	embed.Footer = &discordgo.MessageEmbedFooter{
		Text: fmt.Sprintf("ページ %d / %d", page+1, len(entries)),
	}

	components := []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{
					Label:    "前へ",
					Style:    discordgo.PrimaryButton,
					CustomID: fmt.Sprintf("%s%s:%s:%d", userActivityPrefix, kind, listType, page-1),
					Disabled: page <= 0,
				},
				discordgo.Button{
					Label:    "次へ",
					Style:    discordgo.PrimaryButton,
					CustomID: fmt.Sprintf("%s%s:%s:%d", userActivityPrefix, kind, listType, page+1),
					Disabled: page >= len(entries)-1,
				},
			},
		},
	}
	return embed, components, file, nil
}

func buildUserActivityDetailEmbedFromEntry(kind string, entry userActivityEntry) (*discordgo.MessageEmbed, *discordgo.File) {
	name := formatUserDisplayName(entry.Name, entry.ID)
	alliance := entry.Alliance
	if alliance == "" {
		alliance = "-"
	}
	discordName := entry.Discord
	if discordName == "" {
		discordName = "-"
	}
	discordID := entry.DiscordID
	if discordID == "" {
		discordID = "-"
	}
	lastSeenText := "-"
	if !entry.LastSeen.IsZero() {
		jst := time.FixedZone("JST", 9*3600)
		lastSeenText = entry.LastSeen.In(jst).Format("2006-01-02 15:04:05")
	}
	title := "🚨 荒らしユーザー詳細"
	if kind == userListKindFix {
		title = "🛠️ 修復ユーザー詳細"
	}
	embed := &discordgo.MessageEmbed{
		Title: title,
		Color: 0x3498DB,
		Fields: []*discordgo.MessageEmbedField{
			{Name: "ユーザー", Value: name, Inline: true},
			{Name: "同盟", Value: alliance, Inline: true},
			{Name: "Discord", Value: discordName, Inline: true},
			{Name: "Discord ID", Value: discordID, Inline: true},
			{Name: "荒らし数", Value: fmt.Sprintf("%d", entry.VandalCount), Inline: true},
			{Name: "修復数", Value: fmt.Sprintf("%d", entry.RestoredCount), Inline: true},
			{Name: "スコア", Value: fmt.Sprintf("%d", entry.Score), Inline: true},
			{Name: "最終観測", Value: lastSeenText, Inline: false},
		},
		Timestamp: time.Now().Format(time.RFC3339),
	}

	file := buildUserActivityImageFile(entry)
	if file != nil {
		embed.Image = &discordgo.MessageEmbedImage{
			URL: "attachment://" + file.Name,
		}
	}
	return embed, file
}

func loadUserActivityEntries(dataDir, kind, listType string) ([]userActivityEntry, error) {
	path := filepath.Join(dataDir, "user_activity.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var raw map[string]*activity.UserActivity
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	entries := make([]userActivityEntry, 0, len(raw))
	for id, entry := range raw {
		score := 0
		if listType == userListTypeAbsolute {
			if kind == userListKindFix {
				score = entry.RestoredCount
			} else {
				score = entry.VandalCount
			}
		} else {
			if kind == userListKindFix {
				score = entry.RestoredCount - entry.VandalCount
			} else {
				score = entry.VandalCount - entry.RestoredCount
			}
		}
		if score <= 0 {
			continue
		}
		lastSeen := parseUserListTime(entry.LastSeen)
		entries = append(entries, userActivityEntry{
			ID:            id,
			Name:          entry.Name,
			Alliance:      entry.AllianceName,
			Discord:       entry.Discord,
			DiscordID:     entry.DiscordID,
			Picture:       entry.Picture,
			VandalCount:   entry.VandalCount,
			RestoredCount: entry.RestoredCount,
			Score:         score,
			LastSeen:      lastSeen,
		})
	}
	return entries, nil
}

func loadUserActivityByDiscordName(dataDir, query string) ([]userActivityEntry, error) {
	path := filepath.Join(dataDir, "user_activity.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var raw map[string]*activity.UserActivity
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	queryLower := strings.ToLower(strings.TrimSpace(query))
	if queryLower == "" {
		return nil, nil
	}
	matches := make([]userActivityEntry, 0)
	for id, entry := range raw {
		if entry.Discord == "" {
			continue
		}
		if !strings.Contains(strings.ToLower(entry.Discord), queryLower) {
			continue
		}
		matches = append(matches, userActivityEntry{
			ID:            id,
			Name:          entry.Name,
			Alliance:      entry.AllianceName,
			Discord:       entry.Discord,
			DiscordID:     entry.DiscordID,
			Picture:       entry.Picture,
			VandalCount:   entry.VandalCount,
			RestoredCount: entry.RestoredCount,
			Score:         entry.VandalCount - entry.RestoredCount,
			LastSeen:      parseUserListTime(entry.LastSeen),
		})
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Score == matches[j].Score {
			return matches[i].Discord < matches[j].Discord
		}
		return matches[i].Score > matches[j].Score
	})
	return matches, nil
}

func loadUserActivityByID(dataDir, userID, discordID string) (userActivityEntry, error) {
	path := filepath.Join(dataDir, "user_activity.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return userActivityEntry{}, err
	}
	var raw map[string]*activity.UserActivity
	if err := json.Unmarshal(data, &raw); err != nil {
		return userActivityEntry{}, err
	}
	normalizedDiscord := strings.TrimSpace(discordID)
	normalizedUser := strings.TrimSpace(userID)
	for id, entry := range raw {
		if normalizedUser != "" && id == normalizedUser {
			return userActivityEntry{
				ID:            id,
				Name:          entry.Name,
				Alliance:      entry.AllianceName,
				Discord:       entry.Discord,
				DiscordID:     entry.DiscordID,
				Picture:       entry.Picture,
				VandalCount:   entry.VandalCount,
				RestoredCount: entry.RestoredCount,
				Score:         entry.VandalCount - entry.RestoredCount,
				LastSeen:      parseUserListTime(entry.LastSeen),
			}, nil
		}
		if normalizedDiscord != "" && strings.EqualFold(entry.DiscordID, normalizedDiscord) {
			return userActivityEntry{
				ID:            id,
				Name:          entry.Name,
				Alliance:      entry.AllianceName,
				Discord:       entry.Discord,
				DiscordID:     entry.DiscordID,
				Picture:       entry.Picture,
				VandalCount:   entry.VandalCount,
				RestoredCount: entry.RestoredCount,
				Score:         entry.VandalCount - entry.RestoredCount,
				LastSeen:      parseUserListTime(entry.LastSeen),
			}, nil
		}
	}
	return userActivityEntry{}, fmt.Errorf("該当ユーザーが見つかりません")
}

func decodePictureDataURL(value string) *discordgo.File {
	if value == "" || !strings.HasPrefix(value, "data:image/") {
		return nil
	}
	parts := strings.SplitN(value, ",", 2)
	if len(parts) != 2 {
		return nil
	}
	header := parts[0]
	payload := parts[1]
	if !strings.Contains(header, ";base64") {
		return nil
	}
	data, err := base64.StdEncoding.DecodeString(payload)
	if err != nil || len(data) == 0 {
		return nil
	}
	ext := "png"
	switch {
	case strings.Contains(header, "image/jpeg"):
		ext = "jpg"
	case strings.Contains(header, "image/webp"):
		ext = "webp"
	}
	filename := "user_picture." + ext
	return &discordgo.File{
		Name:        filename,
		ContentType: strings.TrimPrefix(strings.SplitN(header, ";", 2)[0], "data:"),
		Reader:      bytes.NewReader(data),
	}
}

func normalizeUserListKind(kind string) string {
	switch strings.ToLower(kind) {
	case userListKindFix:
		return userListKindFix
	default:
		return userListKindGrf
	}
}

func buildOptionalFiles(file *discordgo.File) []*discordgo.File {
	if file == nil {
		return nil
	}
	return []*discordgo.File{file}
}

func buildUserActivityImageFile(entry userActivityEntry) *discordgo.File {
	if entry.Picture != "" {
		if file := decodePictureDataURL(entry.Picture); file != nil {
			return file
		}
	}
	seed := entry.DiscordID
	if seed == "" {
		seed = entry.ID
	}
	if seed == "" {
		seed = entry.Discord
	}
	if seed == "" {
		return nil
	}
	data, err := buildIdenticonPNG(seed, userActivityIconSize)
	if err != nil {
		return nil
	}
	return &discordgo.File{
		Name:        "user_identicon.png",
		ContentType: "image/png",
		Reader:      bytes.NewReader(data),
	}
}

func buildUserActivitySearchEmbed(query string, entries []userActivityEntry) (*discordgo.MessageEmbed, []discordgo.MessageComponent) {
	limit := len(entries)
	if limit > userActivityMaxSelectItems {
		limit = userActivityMaxSelectItems
	}
	options := make([]discordgo.SelectMenuOption, 0, limit)
	lines := make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		entry := entries[i]
		name := formatUserDisplayName(entry.Name, entry.ID)
		discordName := entry.Discord
		if discordName == "" {
			discordName = "-"
		}
		label := fmt.Sprintf("%s (%s)", discordName, name)
		options = append(options, discordgo.SelectMenuOption{
			Label:       truncateLabel(label, 100),
			Value:       entry.ID,
			Description: truncateLabel(name, 100),
		})
		lines = append(lines, fmt.Sprintf("%d. %s / %s", i+1, discordName, name))
	}
	description := fmt.Sprintf("検索: %s / 候補: %d件", query, len(entries))
	if len(entries) > limit {
		description += fmt.Sprintf("（表示は先頭%d件まで）", limit)
	}
	embed := &discordgo.MessageEmbed{
		Title:       "ユーザー候補を選択",
		Description: description + "\n" + strings.Join(lines, "\n"),
		Color:       0x3498DB,
		Timestamp:   time.Now().Format(time.RFC3339),
	}
	components := []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.SelectMenu{
					CustomID:    userActivitySelectPrefix,
					Placeholder: "ユーザーを選択",
					Options:     options,
				},
			},
		},
	}
	return embed, components
}

func truncateLabel(value string, max int) string {
	if len(value) <= max {
		return value
	}
	if max <= 3 {
		return value[:max]
	}
	return value[:max-3] + "..."
}

func buildIdenticonPNG(seed string, size int) ([]byte, error) {
	if size <= 0 {
		size = userActivityIconSize
	}
	hash := simpleHash(seed)
	hue := float64(hash%userActivityColorsNb) * (360.0 / float64(userActivityColorsNb))
	r, g, b := hslToRGB(hue, float64(userActivitySaturation), float64(userActivityLightness))
	fill := color.RGBA{R: r, G: g, B: b, A: 0xFF}

	img := image.NewRGBA(image.Rect(0, 0, size, size))
	cell := size / 5
	if cell <= 0 {
		cell = 1
	}
	for i := 0; i < 25; i++ {
		if (hash & (1 << (i % 15))) == 0 {
			continue
		}
		x := i / 5
		if i > 14 {
			x = 7 - x
		}
		y := i % 5
		startX := x * cell
		startY := y * cell
		for py := startY; py < startY+cell && py < size; py++ {
			for px := startX; px < startX+cell && px < size; px++ {
				img.Set(px, py, fill)
			}
		}
	}
	buf := new(bytes.Buffer)
	if err := png.Encode(buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func simpleHash(value string) uint32 {
	hash := int32(userActivityMagicNumber)
	for _, r := range value {
		hash = (hash ^ int32(r)) * -userActivityMagicNumber
	}
	return uint32(hash) >> 2
}

func hslToRGB(h, s, l float64) (uint8, uint8, uint8) {
	h = math.Mod(h, 360) / 360.0
	s = clamp01(s / 100.0)
	l = clamp01(l / 100.0)
	if s == 0 {
		v := uint8(math.Round(l * 255))
		return v, v, v
	}
	var q float64
	if l < 0.5 {
		q = l * (1 + s)
	} else {
		q = l + s - l*s
	}
	p := 2*l - q
	r := hueToRGB(p, q, h+1.0/3.0)
	g := hueToRGB(p, q, h)
	b := hueToRGB(p, q, h-1.0/3.0)
	return uint8(math.Round(r * 255)), uint8(math.Round(g * 255)), uint8(math.Round(b * 255))
}

func hueToRGB(p, q, t float64) float64 {
	if t < 0 {
		t += 1
	}
	if t > 1 {
		t -= 1
	}
	switch {
	case t < 1.0/6.0:
		return p + (q-p)*6*t
	case t < 1.0/2.0:
		return q
	case t < 2.0/3.0:
		return p + (q-p)*(2.0/3.0-t)*6
	default:
		return p
	}
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
