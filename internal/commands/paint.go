package commands

import (
	"Koukyo_discord_bot/internal/utils" // 追加
	"fmt"
	"time"

	"github.com/bwmarrin/discordgo"
)

type PaintCommand struct {
	// 通知管理は後で追加
}

func NewPaintCommand() *PaintCommand {
	return &PaintCommand{}
}

func (c *PaintCommand) Name() string { return "paint" }
func (c *PaintCommand) Description() string {
	return "Paint回復時間の計算・通知を行います (30秒/1回復)"
}

func (c *PaintCommand) ExecuteText(s *discordgo.Session, m *discordgo.MessageCreate, args []string) error {
	_, err := s.ChannelMessageSend(m.ChannelID, "このコマンドはスラッシュコマンドで利用してください。")
	return err
}

func (c *PaintCommand) ExecuteSlash(s *discordgo.Session, i *discordgo.InteractionCreate) error {
	var current, max int
	notify := false
	selectedTimezone := "JST" // デフォルトをJSTに設定

	for _, opt := range i.ApplicationCommandData().Options {
		switch opt.Name {
		case "current":
			current = int(opt.IntValue())
		case "max":
			max = int(opt.IntValue())
		case "notify":
			if opt.StringValue() == "on" {
				notify = true
			}
		case "timezone": // timezone オプションを解析
			selectedTimezone = opt.StringValue()
		}
	}
	if current < 0 || max <= 0 || current > max {
		return respond(s, i, "❌ 入力値が不正です (今:0以上, 上限:1以上, 今≦上限)")
	}
	if remain := max - current; remain == 0 {
		return respond(s, i, "🎉 すでに全回復しています！")
	}

	// タイムゾーンをロード
	loc, err := utils.ParseTimezone(selectedTimezone)
	if err != nil {
		return respond(s, i, fmt.Sprintf("❌ 無効なタイムゾーンが指定されました: %s", selectedTimezone))
	}

	// 現在時刻をロードしたタイムゾーンで取得
	nowInLoc := time.Now().In(loc)

	remain := max - current
	recoverSec := remain * 30
	finish := nowInLoc.Add(time.Duration(recoverSec) * time.Second)

	// タイムゾーン情報を含めてフォーマット
	msg := fmt.Sprintf(
		"🖌️ Paint回復計算\n残り: **%d** 回\n全回復まで: **%d分%d秒**\n全回復時刻: **%s (%s)**",
		remain,
		recoverSec/60,
		recoverSec%60,
		finish.Format("15:04:05"), // フォーマットはそのまま、時刻自体が指定TZになる
		finish.Format("MST"),     // タイムゾーン略称を追加
	)
	if notify {
		msg += "\n\n🔔 全回復時に通知します！"
		// 通知管理は後で実装
	}
	return respond(s, i, msg)
}

func (c *PaintCommand) SlashDefinition() *discordgo.ApplicationCommand {
	commonTimezones := utils.GetCommonTimezones()
	timezoneChoices := []*discordgo.ApplicationCommandOptionChoice{}
	for _, tz := range commonTimezones {
		timezoneChoices = append(timezoneChoices, &discordgo.ApplicationCommandOptionChoice{
			Name:  fmt.Sprintf("%s (%s)", tz.Label, tz.Location.String()),
			Value: tz.Name, // ParseTimezone に渡せる値
		})
	}

	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionInteger,
				Name:        "current",
				Description: "現在のPaint数",
				Required:    true,
			},
			{
				Type:        discordgo.ApplicationCommandOptionInteger,
				Name:        "max",
				Description: "Paint上限値",
				Required:    true,
			},
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "notify",
				Description: "全回復時に通知 (on/off)",
				Required:    false,
				Choices: []*discordgo.ApplicationCommandOptionChoice{
					{Name: "on", Value: "on"},
					{Name: "off", Value: "off"},
				},
			},
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "timezone",
				Description: "タイムゾーン (デフォルト: JST)",
				Required:    false,
				Choices:     timezoneChoices,
			},
		},
	}
}

func respond(s *discordgo.Session, i *discordgo.InteractionCreate, msg string) error {
	return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: msg,
		},
	})
}
