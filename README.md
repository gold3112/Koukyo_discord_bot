# Koukyo Discord Bot (Go Edition)

Wplace 監視を行う Discord Bot の Go 実装です。WebSocket の差分監視・通知・ユーザー活動集計・画像生成をまとめて提供します。

## クイックスタート

### 1) 環境変数（推奨: Docker env_file）

リポジトリ外に secrets を置く想定例:

```
D:\Programing\VS_Code\go\secrets\koukyo_discord_bot.env
```

中身:

```env
DISCORD_TOKEN=your_discord_token_here
WEBSOCKET_URL=wss://example.com/ws
MONITOR_POLL_URL=https://example.com/monitor/status
MONITOR_STANDALONE_TARGET_ID=
MONITOR_STANDALONE_ORIGIN=1818-806-989-358
MONITOR_STANDALONE_TEMPLATE=1818-806-989-358.png
POWER_SAVE_MODE=0
```

`docker-compose.yml` からは以下のように参照します:

```yaml
services:
  discord-bot:
    env_file:
      - ../secrets/koukyo_discord_bot.env
```

### 2) Docker 起動

```bash
docker compose up --build
```

### 3) ローカル起動

```bash
go run ./cmd/bot
```

## 主な機能

- WebSocket での差分監視（差分率/加重差分率、画像データ）
- 差分通知（Tier 制、0%復帰/完了通知、ロールメンション対応）
- 小規模差分モード（10px以下）: 1つのテキスト通知を更新し続け、差分座標を高倍率URL付きで表示
- 省電力解除直後の断定補助: 解除直後の急増差分を最初の検出ユーザーに高確率帰属
- サーバー別設定パネル（`/settings`）
- ユーザー活動の追跡/可視化（荒らし/修復のスコア・履歴）
- 画像生成（/now の結合画像、グラフ/ヒートマップ/タイムラプス）
- 地図/タイル取得ユーティリティ（`/get`、`/regionmap`）
- 追加監視（`watch_targets.json`）と進捗監視（`progress_targets.json`）
- WebSocket 断時のフォールバック（HTTP Poll + Standalone監視）
- 外部 API 向けのレートリミッター（既定 2 RPS）

## コマンド一覧

テキストコマンドは `!` プレフィックス、スラッシュコマンドは `/` で利用できます（一部はスラッシュ専用）。
起動時に自動でスラッシュコマンドを同期します。

### 監視・通知
- `now` - 現在の監視状況を表示
- `graph` - 差分率の時系列グラフ（期間指定可）
- `predict` - 修復速度から完全修復までの推定時間を表示
- `timelapse` - 差分率 30%→0.2% のタイムラプス（GIF）
- `heatmap` - 最近の変化量ヒートマップ
- `settings` - 通知/閾値などの設定パネル（管理者向け）
- `notification` - 荒らし/修復ユーザー通知チャンネル設定（管理者向け）
- `progresschannel` - ピクセルアート進捗通知チャンネル設定（管理者向け）
- `status` - Bot 自体の稼働状況（メモリ、稼働時間など）

### ユーザー活動
- `me` - 自分の活動カード表示（Wplace 連携フローあり）
- `useractivity` - ユーザー活動の検索/詳細表示（スラッシュ専用）
- `fixuser` - 修復ユーザー一覧（ランキング/最近、score/absolute）
- `grfuser` - 荒らしユーザー一覧（ランキング/最近、score/absolute）

### 地図・取得系
- `get` - タイル/Region/フルサイズ画像取得（スラッシュ専用）
- `regionmap` - 地域の Region 配置マップ（スラッシュ専用）
- `convert` - 座標変換（経度緯度 ⇄ ピクセル）

### 便利コマンド
- `help` - コマンド一覧
- `info` - Bot 情報
- `ping` - 疎通確認
- `time` - 時刻表示/時差変換
- `paint` - Paint 回復時間の計算（スラッシュ専用）

※ `graph` / `timelapse` / `heatmap` は WebSocket 監視が有効なときのみ利用できます。

## 追加監視 / 進捗監視

### 追加監視（荒らし検知）
- 設定: `data/watch_targets.json`
- 画像: `data/template_img/`
- 手動取得: `!{id}`

### 進捗監視（制作向け）
- 設定: `data/progress_targets.json`
- 画像: `data/template_img/`
- チャンネル設定: `/progresschannel act:on`
- 手動取得: `!{id}`

JSON 形式は共通です:
```json
{
  "targets": [
    {
      "id": "koukyo-main",
      "label": "Koukyo Main",
      "origin": "1818-806-989-358",
      "template": "koukyo_main.png",
      "interval_seconds": 30
    }
  ]
}
```

### 通知ポリシー（重要）

- 追加監視 / 進捗監視の「取得失敗」「テンプレート解決失敗」などは、Discord チャンネルへは送信せずローカルログのみに出力します。
- 誤検知や回線不良で監視ループに影響が出ないよう、通知処理は非同期ディスパッチで監視ループと分離しています。

### 小規模差分（10px以下）の挙動

- 10px 以下の差分は Embed ではなくテキスト通知を更新（編集）して運用します。
- 差分行は `- (tileX-tileY-pixelX-pixelY:URL)` 形式で、高倍率URL（`BuildWplaceHighDetailPixelURL`）を出力します。
- 省電力モードの入退出時は small diff の編集先メッセージ追跡をリセットし、古いメッセージ誤編集を防止します。
- 差分が 10px を超えると large diff 通知に遷移し、スナップショット付き通知を送信します。

## 設定

必須/任意の環境変数:

- `DISCORD_TOKEN` (必須)
- `WEBSOCKET_URL` (任意: 未指定の場合は監視機能が無効)
- `MONITOR_POLL_URL` (任意: WebSocket が1分以上切断された際のHTTPフォールバック取得先)
- `MONITOR_STANDALONE_TARGET_ID` (任意: WS断時の自前監視で使う watch target ID。指定時のみ watch_targets を参照)
- `MONITOR_STANDALONE_ORIGIN` (任意: watch target が解決できない場合のフォールバック座標)
- `MONITOR_STANDALONE_TEMPLATE` (任意: watch target が解決できない場合のフォールバックテンプレート。既定: `1818-806-989-358.png`)
- `POWER_SAVE_MODE` (任意: `1` で起動時に省電力モード)

## 時刻基準

- グラフ (`graph`) は JST 軸で表示します。
- タイムラプス (`timelapse`) の開始/終了時刻は JST で表示します。
- 日次サマリ/日次ランキング集計は JST 日付で処理します。

データは `data/` に保存されます（Docker 利用時は `/app/data`）。

- `data/settings.json`
- `data/user_activity.json`
- `data/vandalized_pixels.json`
- `data/vandal_daily.json`
- `data/achievements.json`
- `data/watch_targets.json` (追加監視ターゲット定義)
- `data/progress_targets.json` (進捗監視ターゲット定義)
- `data/template_img/` (監視用テンプレート画像)

## Discord 側の設定

Bot に以下の Intents を許可してください:

- `Guilds`
- `Guild Messages`
- `Message Content`

※ メッセージコマンドを使わない場合でも、`Message Content` が無効だと `!` コマンドは反応しません。

## 実行方法

ローカル実行:
```bash
go run ./cmd/bot
```

ビルド:
```bash
go build -o bot.exe ./cmd/bot
```

Docker:
```bash
docker compose up --build
```

## パフォーマンス / 安定性メモ

- レートリミッター既定値は 2 RPS（`cmd/bot/main.go` で `NewRateLimiter(2)`）。
- Discord REST クライアントに 15 秒タイムアウトを設定し、通知更新で監視ループが詰まるリスクを低減。
- タイル一括取得はワーカープール方式で実行し、過剰な goroutine 生成を回避。
- small diff 座標抽出はキャッシュし、同一 diff 画像の再デコードを抑制。
- WebSocket テキストメッセージ解析は単一 Unmarshal に統一してオーバーヘッドを削減。

## トラブルシュート

- スラッシュコマンドが出てこない  
  -> Bot の再起動後に同期されます。権限不足や API エラーがある場合はログを確認してください。

- 監視が動かない  
  -> `WEBSOCKET_URL` の設定と接続先の到達性を確認してください。

- 通知が来ない  
  -> `/settings` で通知チャンネルを設定し、`auto_notify` が有効か確認してください。
  -> 進捗通知は `/progresschannel act:on` が必要です。

## GitHub 用メモ

- 設定ファイルは `data/` に置きます（Docker では `/app/data`）
- `data/template_img/` 配下は監視テンプレ画像を保存します

## 移植元

wplace-koukyo-bot (Python)
