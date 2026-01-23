package monitor

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"

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

// Connect WebSocketサーバーに接続
func (m *Monitor) Connect() error {
	log.Printf("Connecting to WebSocket: %s", m.URL)

	conn, _, err := websocket.DefaultDialer.Dial(m.URL, nil)
	if err != nil {
		return err
	}

	m.mu.Lock()
	m.conn = conn
	m.connected = true
	m.mu.Unlock()

	log.Println("WebSocket connected successfully")
	return nil
}

// Start 監視を開始
func (m *Monitor) Start() error {
	if err := m.Connect(); err != nil {
		return err
	}

	go m.receiveLoop()
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

			if err := m.handleMessage(messageType, message); err != nil {
				log.Printf("Message handling error: %v", err)
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
	var data MonitorData
	if err := json.Unmarshal(message, &data); err != nil {
		return err
	}

	// エラーメッセージの場合
	if data.Type == "error" {
		log.Printf("Server error: %s", data.Message)
		return nil
	}

	// メタデータの場合
	if data.Type == "metadata" {
		m.State.UpdateData(&data)
		log.Printf("Updated: Diff=%.2f%%, Weighted=%.2f%%",
			data.DiffPercentage,
			getWeightedValue(data.WeightedDiffPercentage))
	}

	return nil
}

// handleBinaryMessage バイナリメッセージ（画像）を処理
func (m *Monitor) handleBinaryMessage(message []byte) error {
	// 画像データの処理（今後実装）
	log.Printf("Received binary data: %d bytes", len(message))
	return nil
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
