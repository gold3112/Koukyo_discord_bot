package commands

import (
	"Koukyo_discord_bot/internal/embeds"
	"Koukyo_discord_bot/internal/utils"
	"fmt"
	"strings"

	"github.com/bwmarrin/discordgo"
)

type TimeCommand struct{}

func NewTimeCommand() *TimeCommand {
	return &TimeCommand{}
}

func (c *TimeCommand) Name() string {
	return "time"
}

func (c *TimeCommand) Description() string {
	return "現在時刻を表示または時差変換を行います"
}

func (c *TimeCommand) ExecuteText(s *discordgo.Session, m *discordgo.MessageCreate, args []string) error {
	// 引数がない場合は現在時刻を表示
	if len(args) == 0 {
		embed := embeds.BuildTimeEmbed()
		_, err := s.ChannelMessageSendEmbed(m.ChannelID, embed)
		return err
	}

	// 時差変換の引数をパース
	from, to, timeStr := parseTimeArgs(args)

	if from == "" || to == "" {
		// エラーメッセージを送信
		_, err := s.ChannelMessageSend(m.ChannelID,
			"❌ 使用方法: `!time from:JST to:PST time:23:20` または `!time from:JST to:PST`\n"+
				"時刻を省略した場合は現在時刻を使用します。")
		return err
	}

	// 時差変換を実行
	result, err := utils.ConvertTime(from, to, timeStr)
	if err != nil {
		_, e := s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("❌ エラー: %s", err.Error()))
		return e
	}

	// 結果をEmbedで表示
	embed := &discordgo.MessageEmbed{
		Title:       "🌍 時差変換",
		Description: result,
		Color:       0x3498DB,
		Footer: &discordgo.MessageEmbedFooter{
			Text: "時差変換システム",
		},
	}

	_, err = s.ChannelMessageSendEmbed(m.ChannelID, embed)
	return err
}

func (c *TimeCommand) ExecuteSlash(s *discordgo.Session, i *discordgo.InteractionCreate) error {
	from := ""
	to := ""
	timeStr := ""
	dateStr := ""
	if len(i.ApplicationCommandData().Options) == 0 {
		embed := embeds.BuildTimeEmbed()
		return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Embeds: []*discordgo.MessageEmbed{embed},
			},
		})
	}
	for _, opt := range i.ApplicationCommandData().Options {
		switch opt.Name {
		case "from":
			from = opt.StringValue()
		case "to":
			to = opt.StringValue()
		case "time":
			timeStr = opt.StringValue()
		case "date":
			dateStr = opt.StringValue()
		}
	}

	if from == "" || to == "" {
		return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "❌ from/toを指定してください (例: /time from:JST to:PST time:23:20)",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
	}

	// 日付指定があれば timeStr に結合
	if dateStr != "" {
		if timeStr == "" {
			timeStr = "00:00"
		}
		timeStr = dateStr + "T" + timeStr
	}

	result, err := utils.ConvertTime(from, to, timeStr)
	if err != nil {
		return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "❌ エラー: " + err.Error(),
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
	}

	embed := &discordgo.MessageEmbed{
		Title:       "🌍 時差変換",
		Description: result,
		Color:       0x3498DB,
		Footer: &discordgo.MessageEmbedFooter{
			Text: "時差変換システム",
		},
	}
	return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
		},
	})
}

func (c *TimeCommand) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "from",
				Description: "変換元タイムゾーン (例: JST, PST, UTC)",
				Required:    false,
			},
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "to",
				Description: "変換先タイムゾーン (例: JST, PST, UTC)",
				Required:    false,
			},
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "time",
				Description: "時刻 (例: 23:20 または 23:20:00)",
				Required:    false,
			},
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "date",
				Description: "日付 (例: 2026-01-24)",
				Required:    false,
			},
		},
	}
}

// parseTimeArgs 時差変換の引数をパース
// 形式: from:JST to:PST time:23:20 (time は省略可能)
func parseTimeArgs(args []string) (from, to, timeStr string) {
	for _, arg := range args {
		if strings.HasPrefix(arg, "from:") {
			from = strings.TrimPrefix(arg, "from:")
		} else if strings.HasPrefix(arg, "to:") {
			to = strings.TrimPrefix(arg, "to:")
		} else if strings.HasPrefix(arg, "time:") {
			timeStr = strings.TrimPrefix(arg, "time:")
		}
	}
	return
}
