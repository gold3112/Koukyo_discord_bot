package commands

import (
	"Koukyo_discord_bot/internal/embeds"
	"Koukyo_discord_bot/internal/monitor"
	"fmt"

	"github.com/bwmarrin/discordgo"
)

// HeatmapCommand 変化頻度のヒートマップ表示
type HeatmapCommand struct{ mon *monitor.Monitor }

func NewHeatmapCommand(mon *monitor.Monitor) *HeatmapCommand { return &HeatmapCommand{mon: mon} }

func (c *HeatmapCommand) Name() string { return "heatmap" }
func (c *HeatmapCommand) Description() string {
	return "最近の変化が多い箇所をヒートマップで表示します"
}

func (c *HeatmapCommand) ExecuteText(s *discordgo.Session, m *discordgo.MessageCreate, args []string) error {
	return c.respondHeatmap(s, m.ChannelID)
}

func (c *HeatmapCommand) ExecuteSlash(s *discordgo.Session, i *discordgo.InteractionCreate) error {
	// 即時生成
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	}); err != nil {
		return err
	}
	return c.respondHeatmapFollowup(s, i)
}

func (c *HeatmapCommand) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{Name: c.Name(), Description: c.Description()}
}

func (c *HeatmapCommand) respondHeatmap(s *discordgo.Session, channelID string) error {
	if c.mon == nil || !c.mon.State.HasData() {
		_, err := s.ChannelMessageSend(channelID, "まだ監視データがありません。")
		return err
	}
	counts, gw, gh, _, _ := c.mon.State.GetHeatmapSnapshot()
	if counts == nil {
		images := c.mon.GetLatestImages()
		if images != nil && len(images.DiffImage) > 0 {
			_ = c.mon.State.UpdateHeatmapFromDiff(images.DiffImage)
			counts, gw, gh, _, _ = c.mon.State.GetHeatmapSnapshot()
		}
	}
	if counts == nil {
		_, err := s.ChannelMessageSend(channelID, "ヒートマップの集計がまだありません。")
		return err
	}
	pngBuf, err := embeds.BuildHeatmapPNG(counts, gw, gh, gw, gh)
	if err != nil {
		_, e := s.ChannelMessageSend(channelID, fmt.Sprintf("ヒートマップ生成に失敗しました: %v", err))
		return e
	}
	embed := &discordgo.MessageEmbed{
		Title:       "変化頻度ヒートマップ",
		Description: "色が暖色に近いほど変化が多い領域です",
		Color:       0xFF8800,
		Image:       &discordgo.MessageEmbedImage{URL: "attachment://heatmap.png"},
	}
	_, err = s.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
		Embeds: []*discordgo.MessageEmbed{embed},
		Files: []*discordgo.File{{
			Name:        "heatmap.png",
			ContentType: "image/png",
			Reader:      pngBuf,
		}},
	})
	return err
}

func (c *HeatmapCommand) respondHeatmapFollowup(s *discordgo.Session, i *discordgo.InteractionCreate) error {
	if c.mon == nil || !c.mon.State.HasData() {
		_, err := s.FollowupMessageCreate(i.Interaction, false, &discordgo.WebhookParams{
			Content: "まだ監視データがありません。",
		})
		return err
	}
	counts, gw, gh, _, _ := c.mon.State.GetHeatmapSnapshot()
	if counts == nil {
		images := c.mon.GetLatestImages()
		if images != nil && len(images.DiffImage) > 0 {
			_ = c.mon.State.UpdateHeatmapFromDiff(images.DiffImage)
			counts, gw, gh, _, _ = c.mon.State.GetHeatmapSnapshot()
		}
	}
	if counts == nil {
		_, err := s.FollowupMessageCreate(i.Interaction, false, &discordgo.WebhookParams{
			Content: "ヒートマップの集計がまだありません。",
		})
		return err
	}
	pngBuf, err := embeds.BuildHeatmapPNG(counts, gw, gh, gw, gh)
	if err != nil {
		_, e := s.FollowupMessageCreate(i.Interaction, false, &discordgo.WebhookParams{
			Content: fmt.Sprintf("ヒートマップ生成に失敗しました: %v", err),
		})
		return e
	}
	embed := &discordgo.MessageEmbed{
		Title:       "変化頻度ヒートマップ",
		Description: "色が暖色に近いほど変化が多い領域です",
		Color:       0xFF8800,
		Image:       &discordgo.MessageEmbedImage{URL: "attachment://heatmap.png"},
	}
	_, err = s.FollowupMessageCreate(i.Interaction, false, &discordgo.WebhookParams{
		Embeds: []*discordgo.MessageEmbed{embed},
		Files: []*discordgo.File{{
			Name:        "heatmap.png",
			ContentType: "image/png",
			Reader:      pngBuf,
		}},
	})
	return err
}

