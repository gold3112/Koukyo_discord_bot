package monitor

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"log"
	"sync"
	"time"

	"Koukyo_discord_bot/internal/activity"

	"github.com/gorilla/websocket"
)

// Monitor WebSocket監視クライアント
type Monitor struct {
	URL       string
	State     *MonitorState
	conn      *websocket.Conn
	ctx       context.Context
	cancel    context.CancelFunc
	connected bool
	mu        sync.RWMutex
	writeMu   sync.Mutex
	tracker   *activity.Tracker
	lastMu    sync.Mutex
	lastMsgAt time.Time
}

// NewMonitor 新しいMonitorを作成
func NewMonitor(url string) *Monitor {
	ctx, cancel := context.WithCancel(context.Background())
	return &Monitor{
		URL:    url,
		State:  NewMonitorState(),
		ctx:    ctx,
		cancel: cancel,
	}
}

func (m *Monitor) SetActivityTracker(tracker *activity.Tracker) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tracker = tracker
}

// Connect WebSocketサーバーに接続
func (m *Monitor) Connect() error {
	log.Printf("Connecting to WebSocket: %s", m.URL)

	conn, _, err := websocket.DefaultDialer.Dial(m.URL, nil)
	if err != nil {
		return err
	}
	conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	m.mu.Lock()
	m.conn = conn
	m.connected = true
	m.mu.Unlock()
	m.lastMu.Lock()
	m.lastMsgAt = time.Now()
	m.lastMu.Unlock()

	log.Println("WebSocket connected successfully")
	return nil
}

// Start 監視を開始
func (m *Monitor) Start() error {
	if err := m.Connect(); err != nil {
		return err
	}

	go m.receiveLoop()
	go m.pingLoop()
	go m.keepaliveLoop()
	go m.idleWatchLoop()
	return nil
}

// receiveLoop メッセージ受信ループ
func (m *Monitor) receiveLoop() {
	defer func() {
		m.mu.Lock()
		if m.conn != nil {
			m.conn.Close()
		}
		m.connected = false
		m.mu.Unlock()
	}()

	for {
		select {
		case <-m.ctx.Done():
			log.Println("Monitor stopped")
			return
		default:
			m.mu.RLock()
			conn := m.conn
			m.mu.RUnlock()

			if conn == nil {
				time.Sleep(5 * time.Second)
				continue
			}

			conn.SetReadDeadline(time.Now().Add(2 * time.Second))
			messageType, message, err := conn.ReadMessage()
			if err != nil {
				log.Printf("WebSocket read error: %v", err)
				// 再接続処理
				time.Sleep(10 * time.Second)
				if err := m.Connect(); err != nil {
					log.Printf("Reconnect failed: %v", err)
					continue
				}
				log.Println("Reconnected to WebSocket")
				continue
			}

			m.lastMu.Lock()
			m.lastMsgAt = time.Now()
			m.lastMu.Unlock()

			if err := m.handleMessage(messageType, message); err != nil {
				log.Printf("Message handling error: %v", err)
			}
		}
	}
}

func (m *Monitor) pingLoop() {
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.mu.RLock()
			conn := m.conn
			m.mu.RUnlock()
			if conn == nil {
				continue
			}
			deadline := time.Now().Add(5 * time.Second)
			m.writeMu.Lock()
			err := conn.WriteControl(websocket.PingMessage, []byte("ping"), deadline)
			m.writeMu.Unlock()
			if err != nil {
				log.Printf("WebSocket ping error: %v", err)
				m.mu.Lock()
				if m.conn == conn {
					m.conn.Close()
					m.conn = nil
					m.connected = false
				}
				m.mu.Unlock()
			}
		}
	}
}

func (m *Monitor) keepaliveLoop() {
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.mu.RLock()
			conn := m.conn
			m.mu.RUnlock()
			if conn == nil {
				continue
			}
			m.writeMu.Lock()
			err := conn.WriteMessage(websocket.TextMessage, []byte("ping"))
			m.writeMu.Unlock()
			if err != nil {
				log.Printf("WebSocket keepalive error: %v", err)
			}
		}
	}
}

func (m *Monitor) idleWatchLoop() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	log.Printf("WebSocket idle watcher started")

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.lastMu.Lock()
			last := m.lastMsgAt
			m.lastMu.Unlock()
			if last.IsZero() {
				log.Printf("WebSocket idle for 2s: no messages received (no last message)")
				continue
			}
			elapsed := time.Since(last)
			if elapsed >= 2*time.Second {
				log.Printf("WebSocket idle: no messages received for %s", elapsed.Round(time.Millisecond))
			}
		}
	}
}

// handleMessage メッセージを処理
func (m *Monitor) handleMessage(messageType int, message []byte) error {
	switch messageType {
	case websocket.TextMessage:
		return m.handleTextMessage(message)
	case websocket.BinaryMessage:
		return m.handleBinaryMessage(message)
	}
	return nil
}

// handleTextMessage JSONメッセージを処理
func (m *Monitor) handleTextMessage(message []byte) error {
	var raw map[string]interface{}
	if err := json.Unmarshal(message, &raw); err != nil {
		return err
	}

	// エラーメッセージの場合
	if rawType, ok := raw["type"].(string); ok && rawType == "error" {
		if msg, ok := raw["message"].(string); ok {
			log.Printf("Server error: %s", msg)
		} else {
			log.Printf("Server error: %v", raw["message"])
		}
		return nil
	}

	_, hasDiff := raw["diff_percentage"]
	_, hasPixels := raw["diff_pixels"]
	if hasDiff || hasPixels || raw["type"] == "metadata" {
		var data MonitorData
		if err := json.Unmarshal(message, &data); err != nil {
			return err
		}
		m.State.UpdateData(&data)
		log.Printf("Updated: Diff=%.2f%%, Weighted=%.2f%%",
			data.DiffPercentage,
			getWeightedValue(data.WeightedDiffPercentage))
	}

	return nil
}

// handleBinaryMessage バイナリメッセージ（画像）を処理
func (m *Monitor) handleBinaryMessage(message []byte) error {
	// ヘッダーサイズ: 5バイト (type_id: 1バイト + payload_size: 4バイト)
	headerSize := 5
	if len(message) < headerSize {
		log.Printf("Binary message too short: %d bytes", len(message))
		return nil
	}

	typeID := message[0]
	payloadLen := int(binary.LittleEndian.Uint32(message[1:5]))
	if payloadLen < 0 || headerSize+payloadLen > len(message) {
		log.Printf("Binary payload size mismatch: header=%d, total=%d", payloadLen, len(message))
		payloadLen = len(message) - headerSize
	}
	payload := message[headerSize : headerSize+payloadLen]

	log.Printf("Received binary data: %d bytes, type_id=%d, payload_size=%d", len(message), typeID, payloadLen)

	m.mu.RLock()
	tracker := m.tracker
	m.mu.RUnlock()

	// payloadのコピーを作成（元のバッファが上書きされるのを防ぐ）
	payloadCopy := make([]byte, len(payload))
	copy(payloadCopy, payload)

	// 画像データを更新
	m.mu.Lock()
	if m.State.LatestImages == nil {
		m.State.LatestImages = &ImageData{}
	}

	switch typeID {
	case 2: // Live image
		// 先頭に余分な00バイトがある場合は削除
		if len(payloadCopy) > 0 && payloadCopy[0] == 0x00 {
			payloadCopy = payloadCopy[1:]
		}
		m.State.LatestImages.LiveImage = payloadCopy
		m.State.LatestImages.Timestamp = time.Now()
		// 最初の16バイトをログに出力してフォーマットを確認
		if len(payloadCopy) >= 16 {
			log.Printf("Stored live image: %d bytes, header: %X", len(payloadCopy), payloadCopy[:16])
		} else {
			log.Printf("Stored live image: %d bytes", len(payloadCopy))
		}
	case 3: // Diff image
		// 先頭に余分な00バイトがある場合は削除
		if len(payloadCopy) > 0 && payloadCopy[0] == 0x00 {
			payloadCopy = payloadCopy[1:]
		}
		m.State.LatestImages.DiffImage = payloadCopy
		m.State.LatestImages.Timestamp = time.Now()
		if tracker != nil && !m.State.PowerSaveMode {
			tracker.EnqueueDiffImage(payloadCopy)
		} else if tracker != nil {
			log.Printf("activity tracker skipped: power_save_mode=true")
		}
		// 最初の16バイトをログに出力してフォーマットを確認
		if len(payloadCopy) >= 16 {
			log.Printf("Stored diff image: %d bytes, header: %X", len(payloadCopy), payloadCopy[:16])
		} else {
			log.Printf("Stored diff image: %d bytes", len(payloadCopy))
		}
	default:
		log.Printf("Unknown binary type_id: %d", typeID)
	}
	var imagesCopy *ImageData
	if m.State.LatestImages != nil {
		imagesCopy = &ImageData{
			LiveImage: append([]byte(nil), m.State.LatestImages.LiveImage...),
			DiffImage: append([]byte(nil), m.State.LatestImages.DiffImage...),
			Timestamp: m.State.LatestImages.Timestamp,
		}
	}
	m.mu.Unlock()

	if imagesCopy != nil {
		m.State.UpdateImages(imagesCopy)
	}

	return nil
}

// GetLatestData 最新の監視データを取得
func (m *Monitor) GetLatestData() *MonitorData {
	return m.State.GetLatestData()
}

// GetLatestImages 最新の画像データを取得
func (m *Monitor) GetLatestImages() *ImageData {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.State.LatestImages == nil {
		return nil
	}

	// コピーを返す
	images := &ImageData{
		LiveImage: append([]byte(nil), m.State.LatestImages.LiveImage...),
		DiffImage: append([]byte(nil), m.State.LatestImages.DiffImage...),
		Timestamp: m.State.LatestImages.Timestamp,
	}
	return images
}

// Stop 監視を停止
func (m *Monitor) Stop() {
	log.Println("Stopping monitor...")
	m.cancel()

	m.mu.Lock()
	if m.conn != nil {
		m.conn.Close()
	}
	m.mu.Unlock()
}

// IsConnected 接続状態を確認
func (m *Monitor) IsConnected() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.connected
}

// getWeightedValue ポインタからfloat64を取得
func getWeightedValue(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}
