package monitor

import (
	"bytes"
	"image/png"
	"sync"
	"time"
)

// MonitorData WebSocketから受信する監視データ
type MonitorData struct {
	Type                     string    `json:"type"`
	Message                  string    `json:"message,omitempty"`
	DiffPercentage           float64   `json:"diff_percentage"`
	DiffPixels               int       `json:"diff_pixels"`
	WeightedDiffPercentage   *float64  `json:"weighted_diff_percentage"`
	WeightedDiffColor        string    `json:"weighted_diff_color,omitempty"`
	ChrysanthemumDiffPixels  int       `json:"chrysanthemum_diff_pixels"`
	BackgroundDiffPixels     int       `json:"background_diff_pixels"`
	ChrysanthemumTotalPixels int       `json:"chrysanthemum_total_pixels"`
	BackgroundTotalPixels    int       `json:"background_total_pixels"`
	TotalPixels              int       `json:"total_pixels"`
	Timestamp                time.Time `json:"-"`
}

// ImageData 画像データ
type ImageData struct {
	LiveImage []byte
	DiffImage []byte
	Timestamp time.Time
}

// MonitorState 現在の監視状態
type MonitorState struct {
	LatestData          *MonitorData
	LatestImages        *ImageData
	DiffHistory         []DiffRecord
	WeightedDiffHistory []DiffRecord
	ReferencePixels     ReferencePixels
	PowerSaveMode       bool
	PowerSaveRestart    bool
	ZeroDiffStartTime   *time.Time
	// Timelapse recording
	TimelapseActive      bool
	TimelapseFrames      []TimelapseFrame
	LastTimelapseFrames  []TimelapseFrame
	TimelapseStartTime   *time.Time
	TimelapseCompletedAt *time.Time
	lastTimelapseCapture *time.Time
	// Heatmap aggregation (downsampled grid)
	HeatmapGridW   int
	HeatmapGridH   int
	HeatmapCounts  []uint32
	HeatmapSourceW int
	HeatmapSourceH int
	mu             sync.RWMutex
}

// DiffRecord 差分履歴のレコード
type DiffRecord struct {
	Timestamp  time.Time
	Percentage float64
}

// TimelapseFrame タイムラプスのフレーム（差分画像を保持）
type TimelapseFrame struct {
	Timestamp time.Time
	DiffPNG   []byte
}

// ReferencePixels 基準ピクセル数
type ReferencePixels struct {
	Total         int
	Chrysanthemum int
	Background    int
}

// NewMonitorState 新しい監視状態を作成
func NewMonitorState() *MonitorState {
	return &MonitorState{
		DiffHistory:         make([]DiffRecord, 0),
		WeightedDiffHistory: make([]DiffRecord, 0),
		PowerSaveMode:       false,
	}
}

// UpdateData 監視データを更新
func (ms *MonitorState) UpdateData(data *MonitorData) {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	data.Timestamp = time.Now()
	ms.LatestData = data

	// 基準ピクセル数の更新
	if data.ChrysanthemumTotalPixels > 0 {
		ms.ReferencePixels.Chrysanthemum = data.ChrysanthemumTotalPixels
	}
	if data.BackgroundTotalPixels > 0 {
		ms.ReferencePixels.Background = data.BackgroundTotalPixels
	}
	if data.TotalPixels > 0 {
		ms.ReferencePixels.Total = data.TotalPixels
	}

	// 差分履歴の追加
	if !ms.PowerSaveMode {
		record := DiffRecord{
			Timestamp:  data.Timestamp,
			Percentage: data.DiffPercentage,
		}
		ms.DiffHistory = append(ms.DiffHistory, record)

		// 加重差分履歴
		if data.WeightedDiffPercentage != nil {
			weightedRecord := DiffRecord{
				Timestamp:  data.Timestamp,
				Percentage: *data.WeightedDiffPercentage,
			}
			ms.WeightedDiffHistory = append(ms.WeightedDiffHistory, weightedRecord)
		}

		// メモリ管理: 最新20000件のみ保持
		if len(ms.DiffHistory) > 20000 {
			ms.DiffHistory = ms.DiffHistory[len(ms.DiffHistory)-20000:]
		}
		if len(ms.WeightedDiffHistory) > 20000 {
			ms.WeightedDiffHistory = ms.WeightedDiffHistory[len(ms.WeightedDiffHistory)-20000:]
		}
	}

	// ゼロ差分の追跡
	if data.DiffPercentage == 0 {
		if ms.ZeroDiffStartTime == nil {
			now := time.Now()
			ms.ZeroDiffStartTime = &now
		} else {
			elapsed := time.Since(*ms.ZeroDiffStartTime)
			if elapsed >= 15*time.Minute && !ms.PowerSaveMode {
				// 15分間ゼロなら省電力モードで再起動シグナル
				ms.PowerSaveRestart = true
			} else if elapsed >= 10*time.Minute && !ms.PowerSaveMode {
				// 10分間ゼロなら省電力モードへ
				ms.PowerSaveMode = true
			}
		}
	} else {
		// 差分が検出されたら省電力モード解除
		if ms.PowerSaveMode {
			ms.PowerSaveMode = false
		}
		ms.ZeroDiffStartTime = nil
	}

	// タイムラプスの開始/終了判定（閾値: 開始>=30%, 終了<=0.2%）
	if !ms.TimelapseActive && data.DiffPercentage >= 30.0 {
		now := time.Now()
		ms.TimelapseActive = true
		ms.TimelapseFrames = make([]TimelapseFrame, 0, 256)
		ms.TimelapseStartTime = &now
		ms.TimelapseCompletedAt = nil
		ms.lastTimelapseCapture = nil
	}
	if ms.TimelapseActive && data.DiffPercentage <= 0.2 {
		ms.TimelapseActive = false
		ms.LastTimelapseFrames = ms.TimelapseFrames
		now := time.Now()
		ms.TimelapseCompletedAt = &now
		ms.TimelapseFrames = nil
		ms.TimelapseStartTime = nil
		ms.lastTimelapseCapture = nil
	}
}

// UpdateImages 画像データを更新
func (ms *MonitorState) UpdateImages(images *ImageData) {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	ms.LatestImages = images

	// タイムラプス中で、diff画像があり、一定間隔ごとにフレームを追加
	if ms.TimelapseActive && images != nil && len(images.DiffImage) > 0 {
		now := time.Now()
		if ms.lastTimelapseCapture == nil || now.Sub(*ms.lastTimelapseCapture) >= 10*time.Second {
			// PNGバイト列をコピーして保持
			diffCopy := append([]byte(nil), images.DiffImage...)
			ms.TimelapseFrames = append(ms.TimelapseFrames, TimelapseFrame{
				Timestamp: now,
				DiffPNG:   diffCopy,
			})
			ms.lastTimelapseCapture = &now
		}
	}

	// 省電力モード中は画像更新・ヒートマップ集計をスキップ
	if ms.PowerSaveMode {
		return
	}

	// Heatmap集計（常時）: diff PNGをデコードし、非透過ピクセルをカウント
	if images != nil && len(images.DiffImage) > 0 {
		img, err := png.Decode(bytes.NewReader(images.DiffImage))
		if err == nil {
			b := img.Bounds()
			w := b.Dx()
			h := b.Dy()
			if ms.HeatmapGridW == 0 || ms.HeatmapGridH == 0 {
				// 初期化（200x200グリッド）
				ms.HeatmapGridW = 200
				ms.HeatmapGridH = 200
				ms.HeatmapCounts = make([]uint32, ms.HeatmapGridW*ms.HeatmapGridH)
				ms.HeatmapSourceW = w
				ms.HeatmapSourceH = h
			}
			// 画像サイズが変わる場合はリセット
			if w != ms.HeatmapSourceW || h != ms.HeatmapSourceH {
				ms.HeatmapCounts = make([]uint32, ms.HeatmapGridW*ms.HeatmapGridH)
				ms.HeatmapSourceW = w
				ms.HeatmapSourceH = h
			}
			// スキャン（ステップ間引きで負荷軽減）
			stepX := int(float64(w) / 1000.0)
			if stepX < 1 {
				stepX = 1
			}
			stepY := int(float64(h) / 1000.0)
			if stepY < 1 {
				stepY = 1
			}
			for y := b.Min.Y; y < b.Max.Y; y += stepY {
				for x := b.Min.X; x < b.Max.X; x += stepX {
					_, _, _, a := img.At(x, y).RGBA()
					if a > 0 { // 非透過=変化ありとみなす
						gx := int(float64(x) * float64(ms.HeatmapGridW) / float64(w))
						gy := int(float64(y) * float64(ms.HeatmapGridH) / float64(h))
						if gx < 0 {
							gx = 0
						}
						if gy < 0 {
							gy = 0
						}
						if gx >= ms.HeatmapGridW {
							gx = ms.HeatmapGridW - 1
						}
						if gy >= ms.HeatmapGridH {
							gy = ms.HeatmapGridH - 1
						}
						ms.HeatmapCounts[gy*ms.HeatmapGridW+gx]++
					}
				}
			}
		}
	}
}

// GetLatestDiffPercentage 最新の差分率を取得
func (ms *MonitorState) GetLatestDiffPercentage() float64 {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	if ms.LatestData != nil {
		return ms.LatestData.DiffPercentage
	}
	return 0
}

// HasData データを受信済みか
func (ms *MonitorState) HasData() bool {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	return ms.LatestData != nil
}

// GetLatestData 最新データのコピーを取得
func (ms *MonitorState) GetLatestData() *MonitorData {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	if ms.LatestData == nil {
		return nil
	}

	// コピーを返す
	data := *ms.LatestData
	return &data
}

// GetDiffHistory 期間内の差分履歴を取得
func (ms *MonitorState) GetDiffHistory(duration time.Duration, weighted bool) []DiffRecord {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	var src []DiffRecord
	if weighted {
		src = ms.WeightedDiffHistory
	} else {
		src = ms.DiffHistory
	}
	if duration <= 0 {
		// 全件コピー
		out := make([]DiffRecord, len(src))
		copy(out, src)
		return out
	}
	cutoff := time.Now().Add(-duration)
	// 遅延走査でフィルタ
	out := make([]DiffRecord, 0, len(src))
	for _, r := range src {
		if r.Timestamp.After(cutoff) || r.Timestamp.Equal(cutoff) {
			out = append(out, r)
		}
	}
	return out
}

// GetLastTimelapseFrames 直近完了したタイムラプスのフレームを取得
func (ms *MonitorState) GetLastTimelapseFrames() []TimelapseFrame {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	if len(ms.LastTimelapseFrames) == 0 {
		return nil
	}
	out := make([]TimelapseFrame, len(ms.LastTimelapseFrames))
	copy(out, ms.LastTimelapseFrames)
	return out
}

// GetHeatmapSnapshot 集計済みヒートマップのスナップショット
func (ms *MonitorState) GetHeatmapSnapshot() (counts []uint32, gridW, gridH, srcW, srcH int) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	if ms.HeatmapCounts == nil || ms.HeatmapGridW == 0 || ms.HeatmapGridH == 0 {
		return nil, 0, 0, 0, 0
	}
	cp := make([]uint32, len(ms.HeatmapCounts))
	copy(cp, ms.HeatmapCounts)
	return cp, ms.HeatmapGridW, ms.HeatmapGridH, ms.HeatmapSourceW, ms.HeatmapSourceH
}
