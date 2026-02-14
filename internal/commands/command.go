package commands

import (
	"strings"

	"github.com/bwmarrin/discordgo"
)

// Command 統合コマンドインターフェース
type Command interface {
	Name() string
	Description() string
	// テキストコマンド実行
	ExecuteText(s *discordgo.Session, m *discordgo.MessageCreate, args []string) error
	// スラッシュコマンド実行
	ExecuteSlash(s *discordgo.Session, i *discordgo.InteractionCreate) error
	// スラッシュコマンド定義（nilを返すとスラッシュコマンドとして登録されない）
	SlashDefinition() *discordgo.ApplicationCommand
}

// Registry コマンドの登録と管理
type Registry struct {
	commands map[string]Command
}

// NewRegistry 新しいRegistryを作成
func NewRegistry() *Registry {
	return &Registry{
		commands: make(map[string]Command),
	}
}

// Register コマンドを登録
func (r *Registry) Register(cmd Command) {
	r.commands[strings.ToLower(cmd.Name())] = cmd
}

// Get コマンドを取得
func (r *Registry) Get(name string) (Command, bool) {
	cmd, exists := r.commands[strings.ToLower(name)]
	return cmd, exists
}

// All 全てのコマンドを取得
func (r *Registry) All() map[string]Command {
	return r.commands
}

// GetSlashDefinitions スラッシュコマンド定義を取得
func (r *Registry) GetSlashDefinitions() []*discordgo.ApplicationCommand {
	defs := make([]*discordgo.ApplicationCommand, 0)
	for _, cmd := range r.commands {
		if def := cmd.SlashDefinition(); def != nil {
			defs = append(defs, def)
		}
	}
	return defs
}
