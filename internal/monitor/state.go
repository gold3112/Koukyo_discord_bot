package monitor

import (
	"sync"
	"time"
)

// MonitorData WebSocketから受信する監視データ
type MonitorData struct {
	Type                      string    `json:"type"`
	Message                   string    `json:"message,omitempty"`
	DiffPercentage            float64   `json:"diff_percentage"`
	DiffPixels                int       `json:"diff_pixels"`
	WeightedDiffPercentage    *float64  `json:"weighted_diff_percentage"`
	WeightedDiffColor         string    `json:"weighted_diff_color,omitempty"`
	ChrysanthemumDiffPixels   int       `json:"chrysanthemum_diff_pixels"`
	BackgroundDiffPixels      int       `json:"background_diff_pixels"`
	ChrysanthemumTotalPixels  int       `json:"chrysanthemum_total_pixels"`
	BackgroundTotalPixels     int       `json:"background_total_pixels"`
	TotalPixels               int       `json:"total_pixels"`
	Timestamp                 time.Time `json:"-"`
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
	ZeroDiffStartTime   *time.Time
	mu                  sync.RWMutex
}

// DiffRecord 差分履歴のレコード
type DiffRecord struct {
	Timestamp  time.Time
	Percentage float64
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
		} else if time.Since(*ms.ZeroDiffStartTime) >= 10*time.Minute && !ms.PowerSaveMode {
			// 10分間ゼロなら省電力モードへ
			ms.PowerSaveMode = true
			ms.ZeroDiffStartTime = nil
		}
	} else {
		// 差分が検出されたら省電力モード解除
		if ms.PowerSaveMode {
			ms.PowerSaveMode = false
		}
		ms.ZeroDiffStartTime = nil
	}
}

// UpdateImages 画像データを更新
func (ms *MonitorState) UpdateImages(images *ImageData) {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	ms.LatestImages = images
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
