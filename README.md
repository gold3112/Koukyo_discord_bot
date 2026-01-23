# Koukyo Discord Bot (Go Edition)

Wplace監視Discord Botの Go言語による再実装プロジェクト。

## 🎯 プロジェクト目標

Python版 ([wplace-koukyo-bot](https://github.com/gold3112/wplace-koukyo-bot)) の機能を、
よりシンプルで保守しやすいGoコードに移植し、モジュール設計を活かしたファイル管理を実現。

## 📁 プロジェクト構造

```
Koukyo_discord_bot/
├── cmd/
│   └── bot/
│       └── main.go              # エントリーポイント
├── internal/
│   ├── commands/                # コマンド実装
│   │   ├── command.go           # コマンドインターフェース & レジストリ
│   │   ├── ping.go              # pingコマンド
│   │   ├── help.go              # helpコマンド
│   │   ├── info.go              # infoコマンド
│   │   ├── now.go               # nowコマンド
│   │   ├── time.go              # timeコマンド
│   │   ├── convert.go           # convertコマンド
│   │   ├── graph.go             # 差分率グラフコマンド
│   │   ├── timelapse.go         # タイムラプスコマンド
│   │   └── heatmap.go           # ヒートマップコマンド
│   ├── config/
│   │   └── config.go            # 設定管理
│   ├── embeds/
│   │   ├── embeds.go            # Discord埋め込み生成
│   │   ├── graphs.go            # グラフ画像生成
│   │   ├── timelapse.go         # タイムラプスGIF生成
│   │   └── heatmap.go           # ヒートマップPNG生成
│   ├── handler/
│   │   └── handler.go           # イベントハンドラー
│   ├── models/
│   │   └── bot_info.go          # データモデル
│   ├── monitor/
│   │   └── state.go             # 監視状態管理
│   ├── notifications/
│   │   └── notifier.go          # 通知・自動投稿
│   └── utils/
│       ├── coordinator.go       # 座標変換ロジック
│       ├── timezone.go          # タイムゾーン処理
│       └── ratelimiter.go       # レートリミッター（queue制度）
├── data/
│   └── settings.json            # Bot設定
├── docker-compose.yml           # Docker Compose設定
├── Dockerfile                   # Dockerイメージ定義
├── go.mod                       # Go モジュール定義
└── go.sum                       # Go 依存関係チェックサム
```

## ✅ 実装済みコマンド

### 基本コマンド
- `!ping` / `/ping` - Bot疎通確認
- `!help` / `/help` - コマンド一覧表示

### 情報表示
- `!info` / `/info` - Botバージョン・稼働時間表示
- `!now` / `/now` - 現在の監視状況表示（WebSocket連携✅）
- `!time` / `/time` - 世界各地の現在時刻表示

### 座標変換
- `!convert <lng> <lat>` / `/convert lng:<経度> lat:<緯度>`
  - 経度緯度 → タイル座標 & ピクセル座標
- `!convert <TlX-TlY-PxX-PxY>` / `/convert tlx:... tly:... pxx:... pxy:...`
  - ピクセル座標 → 経度緯度
- `/convert coords:<TlX-TlY-PxX-PxY>`
  - ハイフン形式での座標変換

### 可視化・分析系
- `!graph` / `/graph` - 差分率の履歴グラフPNG生成
- `!timelapse` / `/timelapse` - 30%→0.2%のタイムラプスGIF生成
- `!heatmap` / `/heatmap` - 変化量ヒートマップPNG生成

## 🌐 WebSocket監視機能

- WebSocket URL: `wss://gold3112.online/ws`
- リアルタイムでデータ受信・差分率計算
- データ履歴: 最大20,000件保存

### 省電力モード
- 差分率が10分間0% → 省電力モード（画像・ヒートマップ更新停止、通知）
- 15分間0% → 自動再起動（POWER_SAVE_MODE=1で再起動、Docker自動復帰）
- 差分率が再び変動すると即解除

### queue制度（レートリミッター）
- 外部API/Webサイトへのリクエストは3RPS制限
- ホストごとに独立したキューで安全に処理

### 実装済み機能
- WebSocket自動接続・再接続
- バイナリデータのデコード
- リアルタイム差分率の取得
- スレッドセーフな状態管理
- `/now` コマンドでのデータ表示
- 差分率グラフPNG生成
- タイムラプスGIF自動生成・投稿
- ヒートマップPNG生成
- 省電力モード・自動再起動
- レートリミッター（queue制度）

## 📝 移植状況

### Phase 1: 基本コマンド実装 ✅ (100%)
- [x] info - Bot情報表示
- [x] now - 監視ステータス
- [x] time - タイムゾーン表示
- [x] convert - 座標変換
- [x] ping - 疎通確認
- [x] help - コマンド一覧

### Phase 2: 監視・可視化機能 ✅ (100%)
- [x] WebSocket接続
- [x] リアルタイム差分検知
- [x] データ受信・状態管理
- [x] Discord Intents設定
- [x] 画像表示（/now コマンド）
- [x] 差分率グラフPNG生成
- [x] タイムラプスGIF生成・自動投稿
- [x] ヒートマップPNG生成
- [x] 省電力モード・自動再起動
- [x] レートリミッター（queue制度）

### Phase 3: ユーザー追跡機能 📋 (0%)
- [ ] grfusr - 荒らしユーザーリスト
- [ ] fixusr - 修復者ユーザーリスト
- [ ] leaderboard - ランキング表示

### Phase 4: 高度な機能 📋 (0%)
- [ ] 統計分析
- [ ] 予測モデル (LSTM/Hawkes)
- [ ] タイムラプス生成

## 🔧 開発

### レートリミッター利用例
```go
limiter := utils.NewRateLimiter(3) // 3RPS
result, err := limiter.Do(ctx, "example.com", func() (interface{}, error) {
    resp, err := http.Get("https://example.com/api")
    return resp, err
})
```

### ローカルビルド
```bash
go build -o bot.exe ./cmd/bot
```

### 依存関係の追加
```bash
go get <package-name>
go mod tidy
```

## 📜 ライセンス

MIT License

## 移植元

[wplace-koukyo-bot (Python)](https://github.com/gold3112/wplace-koukyo-bot)
