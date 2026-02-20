package version

const (
	// Botのバージョン番号
	Version = "2.0.0"

	// SupportServerURL サポートサーバーのURL
	SupportServerURL = "https://discord.gg/AgzmhFk43Z"
)

// PatchNotes パッチノートの内容
var PatchNotes = []string{
	"endpoint利用の最適化のために、ユーザー断定ロジックを大幅に変更(めっちゃ速い)",
	"10px以下の荒らしの通知を控えめなものに変更",
}
