package monitor

import (
	"bytes"
	"container/ring"
	"image/png"
	"math"
	"sort"
	"sync"
	"time"
)

const (
	// historyLimit 保持する差分履歴の最大数
	historyLimit = 20000
	// timelapseFrameLimit タイムラプスで保持するフレームの最大数
	timelapseFrameLimit = 512
	// zeroDiffEpsilon 0%判定の許容幅
	zeroDiffEpsilon = 0.005
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
	DiffHistory         *ring.Ring
	WeightedDiffHistory *ring.Ring
	ReferencePixels     ReferencePixels
	PowerSaveMode       bool
	PowerSaveRestart    bool
	ZeroDiffStartTime   *time.Time
	// Timelapse recording
	TimelapseActive      bool
	TimelapseFrames      *ring.Ring
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
		DiffHistory:         ring.New(historyLimit),
		WeightedDiffHistory: ring.New(historyLimit),
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
		ms.DiffHistory.Value = DiffRecord{
			Timestamp:  data.Timestamp,
			Percentage: data.DiffPercentage,
		}
		ms.DiffHistory = ms.DiffHistory.Next()

		if data.WeightedDiffPercentage != nil {
			ms.WeightedDiffHistory.Value = DiffRecord{
				Timestamp:  data.Timestamp,
				Percentage: *data.WeightedDiffPercentage,
			}
			ms.WeightedDiffHistory = ms.WeightedDiffHistory.Next()
		}
	}

	// ゼロ差分の追跡
	if isZeroDiff(data.DiffPercentage) {
		if ms.ZeroDiffStartTime == nil {
			now := time.Now()
			ms.ZeroDiffStartTime = &now
		} else if !ms.PowerSaveMode {
			elapsed := time.Since(*ms.ZeroDiffStartTime)
			if elapsed >= 10*time.Minute {
				ms.PowerSaveMode = true
				ms.PowerSaveRestart = true
			}
		}
	} else {
		if ms.PowerSaveMode {
			ms.PowerSaveMode = false
			ms.PowerSaveRestart = false
		}
		ms.ZeroDiffStartTime = nil
	}

	// タイムラプスの開始/終了判定
	if !ms.TimelapseActive && data.DiffPercentage >= 30.0 {
		now := time.Now()
		ms.TimelapseActive = true
		ms.TimelapseFrames = ring.New(timelapseFrameLimit)
		ms.TimelapseStartTime = &now
		ms.TimelapseCompletedAt = nil
		ms.lastTimelapseCapture = nil
	}
	if ms.TimelapseActive && data.DiffPercentage <= 0.2 {
		ms.TimelapseActive = false
		ms.LastTimelapseFrames = ms.collectTimelapseFrames()
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
	ms.LatestImages = images

	// タイムラプス中で、diff画像があり、一定間隔ごとにフレームを追加
	if ms.TimelapseActive && images != nil && len(images.DiffImage) > 0 {
		now := time.Now()
		if ms.lastTimelapseCapture == nil || now.Sub(*ms.lastTimelapseCapture) >= 10*time.Second {
			diffCopy := append([]byte(nil), images.DiffImage...)
			ms.TimelapseFrames.Value = TimelapseFrame{
				Timestamp: now,
				DiffPNG:   diffCopy,
			}
			ms.TimelapseFrames = ms.TimelapseFrames.Next()
			ms.lastTimelapseCapture = &now
		}
	}
	ms.mu.Unlock()

	// 省電力モードチェック
	ms.mu.RLock()
	isPowerSave := ms.PowerSaveMode
	ms.mu.RUnlock()
	if isPowerSave {
		return
	}

	// Heatmap集計を非同期で実行
	if images != nil && len(images.DiffImage) > 0 {
		diffImageCopy := make([]byte, len(images.DiffImage))
		copy(diffImageCopy, images.DiffImage)
		go ms.updateHeatmapAsync(diffImageCopy)
	}
}

// updateHeatmapAsync はヒートマップ集計を非同期で行う
func (ms *MonitorState) updateHeatmapAsync(diffImage []byte) {
	img, err := png.Decode(bytes.NewReader(diffImage))
	if err != nil {
		// 必要であればログ出力
		// log.Printf("failed to decode diff image for heatmap: %v", err)
		return
	}

	b := img.Bounds()
	w, h := b.Dx(), b.Dy()

	ms.mu.Lock()
	defer ms.mu.Unlock()

	if ms.HeatmapGridW == 0 || ms.HeatmapGridW*ms.HeatmapGridH == 0 || w != ms.HeatmapSourceW || h != ms.HeatmapSourceH {
		ms.HeatmapGridW = w
		ms.HeatmapGridH = h
		ms.HeatmapCounts = make([]uint32, w*h)
		ms.HeatmapSourceW = w
		ms.HeatmapSourceH = h
	}

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
			if a > 0 {
				gx := int(float64(x) * float64(ms.HeatmapGridW) / float64(w))
				gy := int(float64(y) * float64(ms.HeatmapGridH) / float64(h))
				if gx >= 0 && gx < ms.HeatmapGridW && gy >= 0 && gy < ms.HeatmapGridH {
					ms.HeatmapCounts[gy*ms.HeatmapGridW+gx]++
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
	data := *ms.LatestData
	return &data
}

// GetDiffHistory 期間内の差分履歴を取得
func (ms *MonitorState) GetDiffHistory(duration time.Duration, weighted bool) []DiffRecord {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	var src *ring.Ring
	if weighted {
		src = ms.WeightedDiffHistory
	} else {
		src = ms.DiffHistory
	}

	out := make([]DiffRecord, 0, src.Len())
	cutoff := time.Now().Add(-duration)

	src.Do(func(p interface{}) {
		if p == nil {
			return
		}
		r := p.(DiffRecord)
		if duration <= 0 || r.Timestamp.IsZero() || r.Timestamp.After(cutoff) || r.Timestamp.Equal(cutoff) {
			out = append(out, r)
		}
	})

	sort.Slice(out, func(i, j int) bool {
		return out[i].Timestamp.Before(out[j].Timestamp)
	})

	return out
}

func (ms *MonitorState) collectTimelapseFrames() []TimelapseFrame {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	if ms.TimelapseFrames == nil {
		return nil
	}
	out := make([]TimelapseFrame, 0, ms.TimelapseFrames.Len())
	ms.TimelapseFrames.Do(func(p interface{}) {
		if p != nil {
			frame := p.(TimelapseFrame)
			if !frame.Timestamp.IsZero() {
				out = append(out, frame)
			}
		}
	})
	sort.Slice(out, func(i, j int) bool {
		return out[i].Timestamp.Before(out[j].Timestamp)
	})
	return out
}

func isZeroDiff(value float64) bool {
	return math.Abs(value) <= zeroDiffEpsilon
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
