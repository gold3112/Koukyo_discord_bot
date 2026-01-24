package commands

import (
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
		}
	}
	if current < 0 || max <= 0 || current > max {
		return respond(s, i, "❌ 入力値が不正です (今:0以上, 上限:1以上, 今≦上限)")
	}
	remain := max - current
	if remain == 0 {
		return respond(s, i, "🎉 すでに全回復しています！")
	}
	recoverSec := remain * 30
	finish := time.Now().Add(time.Duration(recoverSec) * time.Second)
	msg := fmt.Sprintf(
		"🖌️ Paint回復計算\n残り: **%d** 回\n全回復まで: **%d分%d秒**\n全回復時刻: **%s**",
		remain, recoverSec/60, recoverSec%60, finish.Format("15:04:05"),
	)
	if notify {
		msg += "\n\n🔔 全回復時に通知します！"
		// 通知管理は後で実装
	}
	return respond(s, i, msg)
}

func (c *PaintCommand) SlashDefinition() *discordgo.ApplicationCommand {
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
