package version

const (
	// Version Botのバージョン番号
	Version = "1.0.1"

	// SupportServerURL サポートサーバーのURL
	SupportServerURL = "https://discord.gg/AgzmhFk43Z"
)

// PatchNotes パッチノートの内容
var PatchNotes = []string{
	"Wplace監視システム実装",
	"座標変換システム（経度緯度 ↔ ピクセル座標）",
	"タイムゾーン表示（UTC, JST, PST, CET対応）",
	"時差変換機能（from/to指定で任意の時刻変換）",
	"自動通知システム（差分率段階別通知、省電力モード）",
	"テンプレート情報表示",
	"Bot稼働状況表示（稼働時間、メモリ、次回再起動）",
	"ギルド設定パネル",
	"# 現在移植中!!!",
}
