package version

const (
	// Botのバージョン番号
	Version = "2.0.3"

	// SupportServerURL サポートサーバーのURL
	SupportServerURL = "https://discord.gg/AgzmhFk43Z"
)

// PatchNotes パッチノートの内容
var PatchNotes = []string{
	"nowコマンドにWplace.liveリンクと /get fullsize の導線を追加",
	"差分通知Embedに同時検出ユーザー（user#id | xxpx）上位5件を表示",
	"実績ストアでwplaceキーとDiscordキーの分断データを自動マージ",
	"Wplace実装メモ（WPLACE_TECH_MEMO.md）を追加",
}
