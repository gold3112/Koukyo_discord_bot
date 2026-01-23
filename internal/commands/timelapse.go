package commands

import (
	"Koukyo_discord_bot/internal/embeds"
	"Koukyo_discord_bot/internal/monitor"
	"fmt"
	"time"

	"github.com/bwmarrin/discordgo"
)

// TimelapseCommand 閾値(>=30%→<=0.2%)の期間タイムラプス(GIF)を生成
// ローカル保存せず、メモリ生成して送信
type TimelapseCommand struct {
	mon *monitor.Monitor
}

func NewTimelapseCommand(mon *monitor.Monitor) *TimelapseCommand {
	return &TimelapseCommand{mon: mon}
}

func (c *TimelapseCommand) Name() string { return "timelapse" }
func (c *TimelapseCommand) Description() string {
	return "差分率30%→0.2%までのタイムラプスを生成します (GIF)"
}

func (c *TimelapseCommand) ExecuteText(s *discordgo.Session, m *discordgo.MessageCreate, args []string) error {
	return c.respondTimelapse(s, m.ChannelID)
}

func (c *TimelapseCommand) ExecuteSlash(s *discordgo.Session, i *discordgo.InteractionCreate) error {
	// 生成して即座に返す
	if c.mon == nil || !c.mon.State.HasData() {
		return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "まだ監視データがありません。",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
	}

	frames := c.mon.State.GetLastTimelapseFrames()
	if len(frames) == 0 {
		return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "タイムラプス対象の期間が見つかりません。(30%→0.2%)",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
	}

	gifBuf, err := embeds.BuildTimelapseGIF(frames)
	if err != nil {
		return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: fmt.Sprintf("タイムラプス生成に失敗しました: %v", err),
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
	}

	embed := &discordgo.MessageEmbed{
		Title:       "差分タイムラプス",
		Description: fmt.Sprintf("フレーム数: %d / 開始: %s / 終了: %s", len(frames), frames[0].Timestamp.Format("15:04:05"), frames[len(frames)-1].Timestamp.Format("15:04:05")),
		Color:       0x00AA88,
		Timestamp:   time.Now().Format(time.RFC3339),
	}

	return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
			Files: []*discordgo.File{{
				Name:        "timelapse.gif",
				ContentType: "image/gif",
				Reader:      gifBuf,
			}},
		},
	})
}

func (c *TimelapseCommand) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
	}
}

func (c *TimelapseCommand) respondTimelapse(s *discordgo.Session, channelID string) error {
	if c.mon == nil || !c.mon.State.HasData() {
		_, err := s.ChannelMessageSend(channelID, "まだ監視データがありません。")
		return err
	}

	frames := c.mon.State.GetLastTimelapseFrames()
	if len(frames) == 0 {
		_, err := s.ChannelMessageSend(channelID, "タイムラプス対象の期間が見つかりません。(30%→0.2%)")
		return err
	}

	// GIF生成
	gifBuf, err := embeds.BuildTimelapseGIF(frames)
	if err != nil {
		_, e := s.ChannelMessageSend(channelID, fmt.Sprintf("タイムラプス生成に失敗しました: %v", err))
		return e
	}

	// Embed
	embed := &discordgo.MessageEmbed{
		Title:       "差分タイムラプス",
		Description: fmt.Sprintf("フレーム数: %d / 開始: %s / 終了: %s", len(frames), frames[0].Timestamp.Format("15:04:05"), frames[len(frames)-1].Timestamp.Format("15:04:05")),
		Color:       0x00AA88,
		Timestamp:   time.Now().Format(time.RFC3339),
	}

	_, err = s.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
		Embeds: []*discordgo.MessageEmbed{embed},
		Files: []*discordgo.File{{
			Name:        "timelapse.gif",
			ContentType: "image/gif",
			Reader:      gifBuf,
		}},
	})
	return err
}
