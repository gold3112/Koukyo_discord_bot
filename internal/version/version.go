package version

const (
	// Botのバージョン番号
	Version = "2.0.4"

	// SupportServerURL サポートサーバーのURL
	SupportServerURL = "https://discord.gg/AgzmhFk43Z"
)

// PatchNotes パッチノートの内容
var PatchNotes = []string{
	"/dm on|off コマンドを追加：加重差分率10%以上でDM速報を受け取れます",
	"スタンドアロンフォールバックの取得間隔を2秒に短縮",
	"スタンドアロン時に菊のみテンプレートで加重差分を計算",
	"スタンドアロン時にActivityTrackerへ差分画像を連携",
}
