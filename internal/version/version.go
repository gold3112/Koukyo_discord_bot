package version

const (
	// Botのバージョン番号
	Version = "1.6.1-beta"

	// SupportServerURL サポートサーバーのURL
	SupportServerURL = "https://discord.gg/AgzmhFk43Z"
)

// PatchNotes パッチノートの内容
var PatchNotes = []string{
	"荒らし・修復のユーザー追跡をスコア形式として、正負の値で表すように変更しました。",
	"差分率が下がったときのロジックを実装しました。",
	"--現在移植中!!!--",
}
