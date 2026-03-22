package version

const (
	// Botのバージョン番号
	Version = "2.1.0"

	// SupportServerURL サポートサーバーのURL
	SupportServerURL = "https://discord.gg/AgzmhFk43Z"
)

// PatchNotes パッチノートの内容
var PatchNotes = []string{
	"wplace障害検知機能を追加：サーバー障害時・復旧時に通知チャンネルへ自動通知",
	"AllianceID管理を強化：IDによるアライアンス同一性判定に対応（名前偽装対策）",
	"AllianceIDを荒らし/修復ユーザーの記録・日報ランキング・ユーザー検索に反映",
	"/dm on|off コマンドを追加：加重差分率10%以上でDM速報を受け取れます",
}
