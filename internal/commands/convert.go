package commands

import (
	"Koukyo_discord_bot/internal/embeds"
	"fmt"
	"strconv"
	"strings"

	"github.com/bwmarrin/discordgo"
)

type ConvertCommand struct{}

func NewConvertCommand() *ConvertCommand {
	return &ConvertCommand{}
}

func (c *ConvertCommand) Name() string {
	return "convert"
}

func (c *ConvertCommand) Description() string {
	return "座標変換を行います（経度緯度 ⇄ ピクセル座標）"
}

func (c *ConvertCommand) ExecuteText(s *discordgo.Session, m *discordgo.MessageCreate, args []string) error {
	if len(args) == 0 {
		return c.sendUsage(s, m.ChannelID)
	}

	// URLから座標抽出
	if strings.HasPrefix(args[0], "http://") || strings.HasPrefix(args[0], "https://") {
		url := args[0]
		// https://wplace.live/?lat=35.68...&lng=139.75...&zoom=...
		lat, lng := 0.0, 0.0
		latFound, lngFound := false, false
		for _, part := range strings.Split(strings.TrimPrefix(strings.SplitN(url, "?", 2)[1], "/"), "&") {
			if strings.HasPrefix(part, "lat=") {
				if v, err := strconv.ParseFloat(strings.TrimPrefix(part, "lat="), 64); err == nil {
					lat = v
					latFound = true
				}
			}
			if strings.HasPrefix(part, "lng=") {
				if v, err := strconv.ParseFloat(strings.TrimPrefix(part, "lng="), 64); err == nil {
					lng = v
					lngFound = true
				}
			}
		}
		if latFound && lngFound {
			embed := embeds.BuildConvertLngLatEmbed(lng, lat)
			_, err := s.ChannelMessageSendEmbed(m.ChannelID, embed)
			return err
		}
		return c.sendUsage(s, m.ChannelID)
	}

	// ハイフン形式の場合（例: 1818-806-989-358）
	if strings.Contains(args[0], "-") {
		parts := strings.Split(args[0], "-")
		if len(parts) == 4 {
			tileX, _ := strconv.Atoi(parts[0])
			tileY, _ := strconv.Atoi(parts[1])
			pixelX, _ := strconv.Atoi(parts[2])
			pixelY, _ := strconv.Atoi(parts[3])
			embed := embeds.BuildConvertPixelEmbed(tileX, tileY, pixelX, pixelY)
			_, err := s.ChannelMessageSendEmbed(m.ChannelID, embed)
			return err
		}
	}

	// 経度緯度の場合（例: 139.7794 35.6833）
	if len(args) >= 2 {
		lng, err1 := strconv.ParseFloat(args[0], 64)
		lat, err2 := strconv.ParseFloat(args[1], 64)
		if err1 == nil && err2 == nil {
			embed := embeds.BuildConvertLngLatEmbed(lng, lat)
			_, err := s.ChannelMessageSendEmbed(m.ChannelID, embed)
			return err
		}
	}

	return c.sendUsage(s, m.ChannelID)
}

func (c *ConvertCommand) ExecuteSlash(s *discordgo.Session, i *discordgo.InteractionCreate) error {
	options := i.ApplicationCommandData().Options
	optionMap := make(map[string]*discordgo.ApplicationCommandInteractionDataOption)
	for _, opt := range options {
		optionMap[opt.Name] = opt
	}

	// 経度緯度 → ピクセル座標
	if lng, hasLng := optionMap["lng"]; hasLng {
		if lat, hasLat := optionMap["lat"]; hasLat {
			embed := embeds.BuildConvertLngLatEmbed(lng.FloatValue(), lat.FloatValue())
			return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Embeds: []*discordgo.MessageEmbed{embed},
				},
			})
		}
	}

	// ピクセル座標 → 経度緯度
	if tlx, hasTlx := optionMap["tlx"]; hasTlx {
		if tly, hasTly := optionMap["tly"]; hasTly {
			if pxx, hasPxx := optionMap["pxx"]; hasPxx {
				if pxy, hasPxy := optionMap["pxy"]; hasPxy {
					embed := embeds.BuildConvertPixelEmbed(
						int(tlx.IntValue()),
						int(tly.IntValue()),
						int(pxx.IntValue()),
						int(pxy.IntValue()),
					)
					return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
						Type: discordgo.InteractionResponseChannelMessageWithSource,
						Data: &discordgo.InteractionResponseData{
							Embeds: []*discordgo.MessageEmbed{embed},
						},
					})
				}
			}
		}
	}

	// ハイフン形式
	if coords, hasCoords := optionMap["coords"]; hasCoords {
		parts := strings.Split(coords.StringValue(), "-")
		if len(parts) == 4 {
			tileX, _ := strconv.Atoi(parts[0])
			tileY, _ := strconv.Atoi(parts[1])
			pixelX, _ := strconv.Atoi(parts[2])
			pixelY, _ := strconv.Atoi(parts[3])
			embed := embeds.BuildConvertPixelEmbed(tileX, tileY, pixelX, pixelY)
			return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Embeds: []*discordgo.MessageEmbed{embed},
				},
			})
		}
	}

	// 引数不足
	return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: c.getUsageText(),
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
}

func (c *ConvertCommand) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionNumber,
				Name:        "lng",
				Description: "経度 (-180 ~ 180)",
				Required:    false,
			},
			{
				Type:        discordgo.ApplicationCommandOptionNumber,
				Name:        "lat",
				Description: "緯度 (-85 ~ 85)",
				Required:    false,
			},
			{
				Type:        discordgo.ApplicationCommandOptionInteger,
				Name:        "tlx",
				Description: "タイルX座標",
				Required:    false,
			},
			{
				Type:        discordgo.ApplicationCommandOptionInteger,
				Name:        "tly",
				Description: "タイルY座標",
				Required:    false,
			},
			{
				Type:        discordgo.ApplicationCommandOptionInteger,
				Name:        "pxx",
				Description: "ピクセルX座標 (0-999)",
				Required:    false,
			},
			{
				Type:        discordgo.ApplicationCommandOptionInteger,
				Name:        "pxy",
				Description: "ピクセルY座標 (0-999)",
				Required:    false,
			},
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "coords",
				Description: "ハイフン形式 (例: 1818-806-989-358)",
				Required:    false,
			},
		},
	}
}

func (c *ConvertCommand) sendUsage(s *discordgo.Session, channelID string) error {
	_, err := s.ChannelMessageSend(channelID, c.getUsageText())
	return err
}

func (c *ConvertCommand) getUsageText() string {
	return fmt.Sprintf(`❌ 使用方法:
**経度緯度 → ピクセル:** %cconvert <経度> <緯度>%c または %c/convert lng:<経度> lat:<緯度>%c
**ピクセル → 経度緯度:** %cconvert <TlX-TlY-PxX-PxY>%c または %c/convert tlx:<TlX> tly:<TlY> pxx:<PxX> pxy:<PxY>%c
**ハイフン形式:** %c/convert coords:<TlX-TlY-PxX-PxY>%c

例:
%c!convert 139.7794 35.6833%c (東京)
%c!convert 1818-806-989-358%c
%c/convert lng:139.7794 lat:35.6833%c
%c/convert coords:1818-806-989-358%c`,
		'`', '`', '`', '`', '`', '`', '`', '`', '`', '`', '`', '`', '`', '`', '`', '`', '`', '`')
}
