package commands

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"unicode/utf8"

	"github.com/bwmarrin/discordgo"
)

const (
	proxyWebhookName       = "Golden Proxy Relay"
	proxyUsernamePrefix    = "[Golden Proxy]"
	maxWebhookUsernameRune = 80
)

type ProxyCommand struct{}

func NewProxyCommand() *ProxyCommand {
	return &ProxyCommand{}
}

func (c *ProxyCommand) Name() string {
	return "proxy"
}

func (c *ProxyCommand) Description() string {
	return "Webhookで代理投稿します（表示名: [Golden Proxy]{display} (@username / userID)）"
}

func (c *ProxyCommand) ExecuteText(s *discordgo.Session, m *discordgo.MessageCreate, args []string) error {
	if m.GuildID == "" {
		_, err := s.ChannelMessageSend(m.ChannelID, "❌ このコマンドはサーバー内でのみ利用できます。")
		return err
	}
	if !isAdminOrGold(s, m.GuildID, m.Author.ID) {
		_, err := s.ChannelMessageSend(m.ChannelID, "❌ このコマンドは管理者のみ使用できます。")
		return err
	}

	content := strings.TrimSpace(strings.Join(args, " "))
	if content == "" {
		_, err := s.ChannelMessageSend(m.ChannelID, "❌ 使用方法: `!proxy 投稿内容`")
		return err
	}
	if len(content) > 2000 {
		_, err := s.ChannelMessageSend(m.ChannelID, "❌ 文字数が長すぎます（2000文字以内）。")
		return err
	}

	sentMsg, err := c.sendProxyMessage(s, m.ChannelID, m.Author, m.Member, content)
	if err != nil {
		_, sendErr := s.ChannelMessageSend(m.ChannelID, "❌ 代理投稿に失敗しました: "+proxyUserFacingError(err))
		return sendErr
	}
	if sentMsg != nil {
		log.Printf("proxy posted (text): actor_id=%s actor_username=%s channel_id=%s message_id=%s", m.Author.ID, m.Author.Username, m.ChannelID, sentMsg.ID)
	}

	return nil
}

func (c *ProxyCommand) ExecuteSlash(s *discordgo.Session, i *discordgo.InteractionCreate) error {
	if i.GuildID == "" {
		return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "❌ このコマンドはサーバー内でのみ利用できます。",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
	}
	requesterID := ""
	if i.Member != nil && i.Member.User != nil {
		requesterID = i.Member.User.ID
	} else if i.User != nil {
		requesterID = i.User.ID
	}
	if !isAdminOrGold(s, i.GuildID, requesterID) {
		return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "❌ このコマンドは管理者のみ使用できます。",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
	}

	content := ""
	for _, opt := range i.ApplicationCommandData().Options {
		if opt.Name == "message" {
			content = strings.TrimSpace(opt.StringValue())
		}
	}
	if content == "" {
		return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "❌ message を指定してください。",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
	}

	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Flags: discordgo.MessageFlagsEphemeral,
		},
	}); err != nil {
		return err
	}

	var actor *discordgo.User
	if i.Member != nil && i.Member.User != nil {
		actor = i.Member.User
	} else {
		actor = i.User
	}

	sentMsg, err := c.sendProxyMessage(s, i.ChannelID, actor, i.Member, content)
	reply := "✅ 代理投稿しました。"
	if err != nil {
		reply = "❌ 代理投稿に失敗しました: " + proxyUserFacingError(err)
	} else if sentMsg != nil {
		reply = "✅ 代理投稿しました。（message_id: " + sentMsg.ID + "）"
		log.Printf("proxy posted (slash): actor_id=%s actor_username=%s channel_id=%s message_id=%s", actor.ID, actor.Username, i.ChannelID, sentMsg.ID)
	}

	_, editErr := s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: &reply,
	})
	if editErr != nil {
		return editErr
	}
	return nil
}

func (c *ProxyCommand) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "message",
				Description: "代理投稿する本文",
				Required:    true,
				MaxLength:   2000,
			},
		},
	}
}

func (c *ProxyCommand) sendProxyMessage(s *discordgo.Session, channelID string, actor *discordgo.User, member *discordgo.Member, content string) (*discordgo.Message, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, fmt.Errorf("content is empty")
	}
	if actor == nil {
		return nil, fmt.Errorf("actor is nil")
	}

	webhookChannelID := channelID
	threadID := ""

	ch, err := s.Channel(channelID)
	if err != nil {
		return nil, fmt.Errorf("failed to get channel: %w", err)
	}
	if ch != nil && ch.IsThread() {
		if ch.ParentID == "" {
			return nil, fmt.Errorf("thread parent channel is empty")
		}
		webhookChannelID = ch.ParentID
		threadID = channelID
	}

	webhook, err := s.WebhookCreate(webhookChannelID, proxyWebhookName, "")
	if err != nil {
		return nil, fmt.Errorf("failed to create webhook: %w", err)
	}

	defer func() {
		_, delErr := s.WebhookDeleteWithToken(webhook.ID, webhook.Token)
		if delErr != nil {
			log.Printf("proxy webhook cleanup failed: webhook_id=%s err=%v", webhook.ID, delErr)
		}
	}()

	params := &discordgo.WebhookParams{
		Content:   content,
		Username:  buildProxyUsername(member, actor),
		AvatarURL: buildProxyAvatarURL(member, actor),
		AllowedMentions: &discordgo.MessageAllowedMentions{
			Parse: []discordgo.AllowedMentionType{},
		},
	}

	var sentMsg *discordgo.Message
	if threadID != "" {
		sentMsg, err = s.WebhookThreadExecute(webhook.ID, webhook.Token, true, threadID, params)
	} else {
		sentMsg, err = s.WebhookExecute(webhook.ID, webhook.Token, true, params)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to execute webhook: %w", err)
	}

	return sentMsg, nil
}

func buildProxyUsername(member *discordgo.Member, actor *discordgo.User) string {
	displayName := ""
	if member != nil {
		if member.Nick != "" {
			displayName = strings.TrimSpace(member.Nick)
		} else if member.User != nil {
			displayName = strings.TrimSpace(member.User.DisplayName())
		}
	}
	if displayName == "" && actor != nil {
		displayName = strings.TrimSpace(actor.DisplayName())
	}
	if displayName == "" && actor != nil {
		displayName = strings.TrimSpace(actor.Username)
	}
	if displayName == "" {
		displayName = "unknown"
	}

	accountName := "unknown"
	accountID := "unknown"
	if actor != nil {
		if strings.TrimSpace(actor.Username) != "" {
			accountName = strings.TrimSpace(actor.Username)
		}
		if strings.TrimSpace(actor.ID) != "" {
			accountID = strings.TrimSpace(actor.ID)
		}
	}

	nameSuffix := fmt.Sprintf(" (@%s / %s)", accountName, accountID)
	leftPrefix := proxyUsernamePrefix + "{"
	rightPrefix := "}"

	maxDisplayRunes := maxWebhookUsernameRune - utf8.RuneCountInString(leftPrefix) - utf8.RuneCountInString(rightPrefix) - utf8.RuneCountInString(nameSuffix)
	if maxDisplayRunes < 1 {
		maxDisplayRunes = 1
	}

	displayRunes := []rune(displayName)
	if len(displayRunes) > maxDisplayRunes {
		displayName = string(displayRunes[:maxDisplayRunes])
		displayRunes = []rune(displayName)
	}

	name := leftPrefix + displayName + rightPrefix + nameSuffix
	if utf8.RuneCountInString(name) <= maxWebhookUsernameRune {
		return name
	}

	// Fallback: if unexpected wide characters push the length over the limit,
	// trim from the display part again to keep the (@username / userID) suffix.
	for utf8.RuneCountInString(name) > maxWebhookUsernameRune && len(displayRunes) > 0 {
		displayRunes = displayRunes[:len(displayRunes)-1]
		name = leftPrefix + string(displayRunes) + rightPrefix + nameSuffix
	}
	return name
}

func buildProxyAvatarURL(member *discordgo.Member, actor *discordgo.User) string {
	if member != nil {
		if member.User != nil && member.GuildID != "" {
			if url := member.AvatarURL("256"); url != "" {
				return url
			}
		}
		if member.User != nil {
			if url := member.User.AvatarURL("256"); url != "" {
				return url
			}
		}
	}
	if actor != nil {
		if url := actor.AvatarURL("256"); url != "" {
			return url
		}
	}
	return ""
}

func proxyUserFacingError(err error) string {
	if err == nil {
		return "unknown error"
	}
	var restErr *discordgo.RESTError
	if errors.As(err, &restErr) {
		if restErr.Response != nil && restErr.Response.StatusCode == 403 {
			return "権限が不足しています。Botに「Webhookの管理」権限を付与してください。"
		}
		if restErr.Message != nil && restErr.Message.Message != "" {
			return restErr.Message.Message
		}
	}
	return err.Error()
}
