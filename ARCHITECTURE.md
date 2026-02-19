# 技術ドキュメント - Koukyo Discord Bot (Go Edition)

## 概要

本Botは `wplace` の差分監視結果を Discord に通知する Go 実装です。  
WebSocket 監視を主経路にしつつ、回線不良時には HTTP poll / standalone 取得へ段階的にフォールバックします。

## システム構成

```
cmd/bot/main.go
  -> internal/monitor      (WS受信・状態管理・履歴/日次集計)
  -> internal/notifications (通知判定・送信ディスパッチ・日次配信)
  -> internal/activity      (ユーザー活動集計)
  -> internal/handler       (テキスト/スラッシュコマンド分岐)
  -> internal/commands      (各コマンド実装)
  -> internal/embeds        (画像/Embed生成)
  -> internal/wplace        (タイル取得・画像合成)
  -> internal/utils         (座標変換・URL生成・レート制御など)
```

## 起動フロー

1. `cmd/bot/main.go` が設定を読み込み、`SettingsManager` を初期化。
2. `RateLimiter` を2系統で初期化（監視系/活動系とも既定 `2 RPS`）。
3. `Monitor` を起動し、WSループ群を開始。
4. `Notifier.StartMonitoring()` を開始。
5. Discord イベントハンドラ（メッセージ/スラッシュ）を登録して接続。

## Monitor層

主要ファイル: `internal/monitor/monitor.go`, `internal/monitor/state.go`

### 役割

- WebSocket のテキスト/バイナリ受信
- 差分データ・画像の最新状態管理
- 差分履歴、タイムラプスフレーム、ヒートマップ、日次サマリ保持
- 電文断検知と再接続制御

### 常駐ループ

- `receiveLoop`: 受信本体。切断時は指数バックオフ再接続。
- `pingLoop`: WS Ping 制御。
- `keepaliveLoop`: テキスト keepalive。
- `idleWatchLoop`: 長時間無受信を検知し強制再接続。
- `pollFallbackLoop`: WS 断が一定時間継続した時に HTTP から監視データ取得。

### 実装ポイント

- テキスト受信は `monitorTextPayload` で単一 Unmarshal。
- `MonitorState` は `RWMutex` で保護。
- 日次ピーク画像・日次集計は JST 日付キーで保存。
- 直近7日分の `DailySummaries` を保持しメモリ上限を管理。

## Notification層

主要ファイル: `internal/notifications/notifier.go`, `internal/notifications/notifier_monitoring.go`

### 役割

- サーバーごとの通知判定（Tier上昇/下降、0%復帰/完了）
- small diff 特別フロー（テキスト更新）
- 追加監視 / 進捗監視 / 日次ランキング / タイムラプス自動投稿
- 送信処理の非同期化

### ディスパッチ設計

- `dispatchHigh`: 高優先度キュー（FIFO）
- `dispatchLow`: 低優先度キュー（キー単位でcoalescing）
- キュー飽和時は通知をドロップし、監視ループを止めない

### small diff フロー

- 条件: `DiffPixels` が `1..10`（`smallDiffPixelLimit=10`）
- Embedではなく単一テキスト通知を `edit` で更新
- 差分座標を `- (tileX-tileY-pixelX-pixelY:URL)` 形式で列挙
- URLは `BuildWplaceHighDetailPixelURL`（`/me` と同系統の高倍率リンク）
- 省電力モードの入退出時にメッセージ追跡IDをリセットし、古い通知の誤編集を防止

### 省電力解除時の断定補助

- `PowerSaveMode=true -> false` の遷移時に、解除直後の `DiffPixels` を推定対象として arm
- 2px 以上の急増時は、最初に検出された painter を対象ピクセルの主担当として帰属
- 対象ピクセルは短時間TTL内で消費し、通常集計への二重加算を回避

### フォールバック監視

主要ファイル: `internal/notifications/notifier_standalone_fallback.go`

- WS断が1分継続時に standalone 監視へ切替
- ターゲット解決順:
  - `MONITOR_STANDALONE_TARGET_ID`（watch_targets参照）
  - `MONITOR_STANDALONE_ORIGIN` / `MONITOR_STANDALONE_TEMPLATE`
  - 既定値
- standalone失敗時は指数バックオフ

## 追加監視 / 進捗監視

主要ファイル:
- `internal/notifications/notifier_watch_targets.go`
- `internal/notifications/notifier_progress_targets.go`
- `internal/notifications/target_common.go`

### 仕様

- `watch_targets.json` / `progress_targets.json` をTTL付きで再読込
- テンプレート画像は `template_img` から読み込み、更新時刻でキャッシュ
- ターゲット取得は並列数上限付きで実行

### エラー通知方針

- 回線不良時のノイズ抑制のため、追加監視/進捗監視のエラーは Discord 通知しない
- エラーはローカルログへ出力（`suppressed error notification`）

## 画像取得/合成層

主要ファイル: `internal/wplace/tiles.go`

### 仕様

- タイルHTTP取得は接続プールを持つ専用 `http.Client`
- タイルキャッシュ（TTL 2分）
- グリッド取得はワーカープール方式で実行
- `CombineTilesCroppedImage` で必要範囲のみ合成

### 最適化

- 以前の「タイルごとにgoroutine生成」から固定ワーカー数へ変更
- キャッシュロックを `RWMutex` 化し read-heavy パスを軽量化

## 時刻/日次処理

- グラフ時刻軸: JST
- タイムラプス表示時刻: JST
- 日次ランキング送信タイミング: JST日付境界
- 日次サマリ集計キー: JST

主要ファイル:
- `internal/commands/graph.go`
- `internal/commands/timelapse.go`
- `internal/embeds/graphs.go`
- `internal/notifications/notifier_daily_ranking.go`

## マルチギルド配信

日次ランキングの添付画像はギルドごとに個別送信されます。  
同一バッファの使い回しで「最初の1サーバーのみ添付される」問題を回避するため、送信ごとに `Reader` を作り直す実装です。

## データ保存

主な永続ファイル:

- `data/settings.json`
- `data/user_activity.json`
- `data/vandalized_pixels.json`
- `data/vandal_daily.json`
- `data/watch_targets.json`
- `data/progress_targets.json`
- `data/template_img/*`

## テスト

主要テスト対象:

- 座標/URL変換: `internal/utils/*_test.go`
- small diff 座標抽出: `internal/notifications/notifier_small_diff_coords_test.go`
- 日次ランキング/画像添付: `internal/notifications/notifier_daily_ranking_test.go`
- グラフJST表示: `internal/embeds/graphs_test.go`
- monitorテキストpayload解析: `internal/monitor/monitor_text_payload_test.go`

---

最終更新: 2026-02-19
