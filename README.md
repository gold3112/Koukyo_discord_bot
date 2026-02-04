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
- サーバー別設定パネル（`/settings`）
- ユーザー活動の追跡/可視化（荒らし/修復のスコア・履歴）
- 画像生成（/now の結合画像、グラフ/ヒートマップ/タイムラプス）
- 地図/タイル取得ユーティリティ（`/get`、`/regionmap`）
- 外部 API 向けのレートリミッター

## コマンド一覧

テキストコマンドは `!` プレフィックス、スラッシュコマンドは `/` で利用できます（一部はスラッシュ専用）。
起動時に自動でスラッシュコマンドを同期します。

### 監視・通知
- `now` - 現在の監視状況を表示
- `graph` - 差分率の時系列グラフ（期間指定可）
- `timelapse` - 差分率 30%→0.2% のタイムラプス（GIF）
- `heatmap` - 最近の変化量ヒートマップ
- `settings` - 通知/閾値などの設定パネル（管理者向け）
- `notification` - 荒らし/修復ユーザー通知チャンネル設定（管理者向け）
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

## 設定

必須/任意の環境変数:

- `DISCORD_TOKEN` (必須)
- `WEBSOCKET_URL` (任意: 未指定の場合は監視機能が無効)
- `POWER_SAVE_MODE` (任意: `1` で起動時に省電力モード)

データは `data/` に保存されます（Docker 利用時は `/app/data`）。

- `data/settings.json`
- `data/user_activity.json`
- `data/vandalized_pixels.json`
- `data/vandal_daily.json`
- `data/achievements.json`
- `data/watch_targets.json` (追加監視ターゲット定義)
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

## トラブルシュート

- スラッシュコマンドが出てこない  
  -> Bot の再起動後に同期されます。権限不足や API エラーがある場合はログを確認してください。

- 監視が動かない  
  -> `WEBSOCKET_URL` の設定と接続先の到達性を確認してください。

- 通知が来ない  
  -> `/settings` で通知チャンネルを設定し、`auto_notify` が有効か確認してください。

## 移植元

wplace-koukyo-bot (Python)
