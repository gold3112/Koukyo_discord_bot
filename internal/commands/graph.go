package commands

import (
	"Koukyo_discord_bot/internal/embeds"
	"Koukyo_discord_bot/internal/monitor"
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

// GraphCommand 差分率のグラフ表示
type GraphCommand struct {
	mon *monitor.Monitor
}

func NewGraphCommand(mon *monitor.Monitor) *GraphCommand {
	return &GraphCommand{mon: mon}
}

func (c *GraphCommand) Name() string { return "graph" }
func (c *GraphCommand) Description() string {
	return "差分率の時系列グラフを表示します（オプションで期間指定可）"
}

// execute は、グラフ生成の共通ロジック
func (c *GraphCommand) execute(metric string, duration time.Duration) (*discordgo.MessageEmbed, *bytes.Buffer, error) {
	if c.mon == nil || !c.mon.State.HasData() {
		return nil, nil, fmt.Errorf("まだ監視データがありません。")
	}

	weighted := (metric == "weighted")
	history := c.mon.State.GetDiffHistory(duration, weighted)
	pngBuf, err := embeds.BuildDiffGraphPNG(history)
	if err != nil {
		return nil, nil, fmt.Errorf("グラフ生成に失敗しました: %w", err)
	}

	title := "差分率グラフ"
	if weighted {
		title = "加重差分率グラフ"
	}
	embed := &discordgo.MessageEmbed{
		Title:       title,
		Description: fmt.Sprintf("範囲: %s / データ点: %d", humanDuration(duration), len(history)),
		Color:       0x63A4FF,
		Timestamp:   time.Now().Format(time.RFC3339),
		Image:       &discordgo.MessageEmbedImage{URL: "attachment://diff_graph.png"},
	}

	return embed, pngBuf, nil
}

func (c *GraphCommand) ExecuteText(s *discordgo.Session, m *discordgo.MessageCreate, args []string) error {
	metric := "overall"
	duration := 1 * time.Hour

	// 引数: metric=overall|weighted, duration=30m|1h|6h|24h
	for _, a := range args {
		if strings.HasPrefix(a, "metric=") {
			v := strings.TrimPrefix(a, "metric=")
			if v == "weighted" {
				metric = "weighted"
			}
		} else if strings.HasPrefix(a, "duration=") {
			v := strings.TrimPrefix(a, "duration=")
			if d, err := parseDuration(v); err == nil {
				duration = d
			}
		}
	}

	embed, pngBuf, err := c.execute(metric, duration)
	if err != nil {
		_, e := s.ChannelMessageSend(m.ChannelID, err.Error())
		return e
	}

	_, err = s.ChannelMessageSendComplex(m.ChannelID, &discordgo.MessageSend{
		Embeds: []*discordgo.MessageEmbed{embed},
		Files: []*discordgo.File{{
			Name:        "diff_graph.png",
			ContentType: "image/png",
			Reader:      pngBuf,
		}},
	})
	return err
}

func (c *GraphCommand) ExecuteSlash(s *discordgo.Session, i *discordgo.InteractionCreate) error {
	metric := "overall"
	duration := 1 * time.Hour
	opts := i.ApplicationCommandData().Options
	for _, opt := range opts {
		switch opt.Name {
		case "metric":
			v := opt.StringValue()
			if v == "weighted" {
				metric = "weighted"
			}
		case "duration":
			v := opt.StringValue()
			if d, err := parseDuration(v); err == nil {
				duration = d
			}
		}
	}

	embed, pngBuf, err := c.execute(metric, duration)
	if err != nil {
		return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: err.Error(),
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
	}

	return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
			Files: []*discordgo.File{{
				Name:        "diff_graph.png",
				ContentType: "image/png",
				Reader:      pngBuf,
			}},
		},
	})
}

func (c *GraphCommand) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "metric",
				Description: "指標: overall | weighted",
				Required:    false,
				Choices: []*discordgo.ApplicationCommandOptionChoice{
					{Name: "overall", Value: "overall"},
					{Name: "weighted", Value: "weighted"},
				},
			},
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "duration",
				Description: "範囲: 30m | 1h | 6h | 24h",
				Required:    false,
			},
		},
	}
}

func parseDuration(s string) (time.Duration, error) {
	switch s {
	case "30m":
		return 30 * time.Minute, nil
	case "1h":
		return 1 * time.Hour, nil
	case "6h":
		return 6 * time.Hour, nil
	case "24h":
		return 24 * time.Hour, nil
	}
	return time.ParseDuration(s)
}

func humanDuration(d time.Duration) string {
	if d%time.Hour == 0 {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dm", int(d.Minutes()))
}
