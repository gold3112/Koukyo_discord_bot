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
│   │   └── convert.go           # convertコマンド
│   ├── config/
│   │   └── config.go            # 設定管理
│   ├── embeds/
│   │   └── embeds.go            # Discord埋め込みメッセージ生成
│   ├── handler/
│   │   └── handler.go           # イベントハンドラー
│   ├── models/
│   │   └── bot_info.go          # データモデル
│   └── utils/
│       ├── coordinator.go       # 座標変換ロジック
│       └── timezone.go          # タイムゾーン処理
├── .env                         # 環境変数 (Git管理外)
├── .env.example                 # 環境変数サンプル
├── docker-compose.yml           # Docker Compose設定
├── Dockerfile                   # Dockerイメージ定義
├── go.mod                       # Go モジュール定義
└── go.sum                       # Go 依存関係チェックサム

```

## ✅ 実装済みコマンド

### 基本コマンド
- **`!ping` / `/ping`** - Bot疎通確認
- **`!help` / `/help`** - コマンド一覧表示

### 情報表示
- **`!info` / `/info`** - Botバージョン・稼働時間表示
- **`!now` / `/now`** - 現在の監視状況表示（WebSocket連携✅）
- **`!time` / `/time`** - 世界各地の現在時刻表示

### 座標変換
- **`!convert <lng> <lat>` / `/convert lng:<経度> lat:<緯度>`**
  - 経度緯度 → タイル座標 & ピクセル座標
- **`!convert <TlX-TlY-PxX-PxY>` / `/convert tlx:... tly:... pxx:... pxy:...`**
  - ピクセル座標 → 経度緯度
- **`/convert coords:<TlX-TlY-PxX-PxY>`**
  - ハイフン形式での座標変換

## 🌐 WebSocket監視機能 (Phase 2)

### 現在の状態: ✅ **稼働中**

- **WebSocket URL**: `wss://gold3112.online/ws`
- **接続状態**: リアルタイムでデータ受信中
- **差分率**: 0.03%（最新値）
- **加重差分率**: 0.05%（菊重視）
- **データ履歴**: 最大20,000件保存

### 実装済み機能
- ✅ WebSocket自動接続・再接続
- ✅ バイナリデータのデコード
- ✅ リアルタイム差分率の取得
- ✅ スレッドセーフな状態管理
- ✅ `/now` コマンドでのデータ表示

### 改善予定
- 🚧 画像の並列表示（ライブ+差分）
- 🚧 詳細ピクセル情報（菊/背景の内訳）
- 📋 通知システム（閾値設定）

#### 座標変換の例
```
!convert 139.7794 35.6833          # 東京の座標をピクセルに変換
!convert 1818-806-989-358          # ハイフン形式
/convert lng:139.7794 lat:35.6833  # スラッシュコマンド
```

## 🚀 セットアップ & 実行

### 1. 環境変数の設定

```bash
cp .env.example .env
# .env ファイルに Discord Bot Token を設定
```

### 2. Docker で起動

```bash
# ビルド & 起動
docker-compose up -d

# ログ確認
docker-compose logs -f

# 停止
docker-compose down
```

## 🎨 設計思想

### モジュラー設計
- **commands**: 各コマンドを独立したファイルに分離
- **utils**: 座標変換やタイムゾーン処理など汎用ロジック
- **embeds**: Discord埋め込みメッセージ生成を一元管理
- **models**: データ構造を明確に定義

### 堅牢性
- 型安全性を活かしたエラーハンドリング
- インターフェースを使った拡張性の確保
- テキストコマンドとスラッシュコマンドの統一的な処理

### 保守性
- 機能ごとにファイルを分割し、責任を明確化
- コメントによる丁寧な説明
- 命名規則の統一

## 📝 移植状況

### Phase 1: 基本コマンド実装 ✅ (100%)
- [x] info - Bot情報表示
- [x] now - 監視ステータス（仮実装）
- [x] time - タイムゾーン表示
- [x] convert - 座標変換
- [x] ping - 疎通確認
- [x] help - コマンド一覧

### Phase 2: WebSocket監視機能 🚧 (50%)
- [x] WebSocket接続 ✅
- [x] リアルタイム差分検知 ✅
- [x] データ受信・状態管理 ✅
- [x] Discord Intents設定 ✅
- [ ] 画像表示（/now コマンド）🚧
- [ ] 詳細情報表示（/info コマンド）🚧
- [ ] 通知システム 📋

### Phase 3: ユーザー追跡機能 📋 (0%)
- [ ] grfusr - 荒らしユーザーリスト
- [ ] fixusr - 修復者ユーザーリスト
- [ ] leaderboard - ランキング表示

### Phase 4: 高度な機能 📋 (0%)
- [ ] 統計分析
- [ ] グラフ生成
- [ ] 予測モデル (LSTM/Hawkes)
- [ ] タイムラプス生成

## 🔧 開発

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
