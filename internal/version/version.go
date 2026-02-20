package version

const (
	// Botのバージョン番号
	Version = "2.0.2"

	// SupportServerURL サポートサーバーのURL
	SupportServerURL = "https://discord.gg/AgzmhFk43Z"
)

// PatchNotes パッチノートの内容
var PatchNotes = []string{
	"実績ルールJSONを拡張（Guardian/Destroyer/AreYouSleepy?/3日坊主）",
	"実績通知は起動時の初回評価をベースライン同期扱いにしてスパムを抑止",
	"Discord未連携ユーザーでもゲーム内ユーザー基準で実績を付与可能に",
	"proxyコマンドはチャンネル単位でWebhookを再利用し、Webhook由来IDの変動を抑制",
}
