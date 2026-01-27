package version

const (
	// Botのバージョン番号
	Version = "1.7.1-beta"

	// SupportServerURL サポートサーバーのURL
	SupportServerURL = "https://discord.gg/AgzmhFk43Z"
)

// PatchNotes パッチノートの内容
var PatchNotes = []string{
	"user検索ができるように!",
	"荒らしの検出精度を向上、また保存する内容の強化を行いました。",
	"特定の条件下でデッドロックが発生する問題を修正しました。",
	"荒らし・修復のユーザー追跡をスコア形式として、正負の値で表すように変更しました。",
	"差分率が下がったときのロジックを実装しました。",
	"--現在移植中!!!--",
}
