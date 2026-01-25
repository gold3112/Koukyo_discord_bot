package activity

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"image/png"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"time"

	"Koukyo_discord_bot/internal/utils"
)

type Config struct {
	TopLeftTileX  int
	TopLeftTileY  int
	TopLeftPixelX int
	TopLeftPixelY int
	Width         int
	Height        int
}

type Pixel struct {
	AbsX int
	AbsY int
}

type PaintedBy struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	AllianceName string `json:"allianceName"`
}

type PixelAPIResponse struct {
	PaintedBy *PaintedBy `json:"paintedBy"`
}

type PixelRef struct {
	X int `json:"x"`
	Y int `json:"y"`
}

type UserActivity struct {
	ID                  string         `json:"id"`
	Name                string         `json:"name"`
	AllianceName        string         `json:"allianceName"`
	LastSeen            string         `json:"last_seen"`
	VandalCount         int            `json:"vandal_count"`
	RestoredCount       int            `json:"restored_count"`
	ActivityScore       int            `json:"activity_score"`
	DailyVandalCounts   map[string]int `json:"daily_vandal_counts,omitempty"`
	DailyRestoredCounts map[string]int `json:"daily_restored_counts,omitempty"`
	DailyActivityScores map[string]int `json:"daily_activity_scores,omitempty"`
	LastPixel           *PixelRef      `json:"last_pixel,omitempty"`
}

type VandalState struct {
	VandalizedPixels [][]int           `json:"vandalized_pixels"`
	PixelToPainter   map[string]string `json:"pixel_to_painter"`
}

type Tracker struct {
	cfg          Config
	limiter      *utils.RateLimiter
	dataDir      string
	httpClient   *http.Client
	newUserCB    NewUserCallback
	queue        chan Pixel
	diffQueue    chan []byte
	ctx          context.Context
	cancel       context.CancelFunc
	mu           sync.Mutex
	currentDiff  map[string]Pixel
	activity     map[string]*UserActivity
	vandalState  VandalState
	backoffDelay time.Duration
	backoffUntil time.Time
}

type NewUserCallback func(kind string, user UserActivity)

func NewTracker(cfg Config, limiter *utils.RateLimiter, dataDir string) *Tracker {
	ctx, cancel := context.WithCancel(context.Background())
	queueSize := cfg.Width * cfg.Height
	if queueSize <= 0 {
		queueSize = 4096
	}
	diffQueueSize := 1
	dialer := &net.Dialer{
		Timeout:   5 * time.Second,
		KeepAlive: 5 * time.Second,
	}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialer.DialContext(ctx, "tcp4", addr)
		},
		ForceAttemptHTTP2:  false,
		DisableKeepAlives:  true,
		DisableCompression: true,
		TLSNextProto:       make(map[string]func(string, *tls.Conn) http.RoundTripper),
	}
	t := &Tracker{
		cfg:          cfg,
		limiter:      limiter,
		dataDir:      dataDir,
		httpClient:   &http.Client{Transport: transport, Timeout: 8 * time.Second},
		queue:        make(chan Pixel, queueSize),
		diffQueue:    make(chan []byte, diffQueueSize),
		ctx:          ctx,
		cancel:       cancel,
		currentDiff:  make(map[string]Pixel),
		activity:     make(map[string]*UserActivity),
		vandalState:  VandalState{PixelToPainter: make(map[string]string)},
		backoffDelay: 2 * time.Second,
	}
	t.loadState()
	return t
}

func (t *Tracker) SetNewUserCallback(cb NewUserCallback) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.newUserCB = cb
}

func (t *Tracker) Start() {
	go t.worker()
	go t.diffWorker()
}

func (t *Tracker) Stop() {
	t.cancel()
}

func (t *Tracker) EnqueueDiffImage(pngBytes []byte) {
	if len(pngBytes) == 0 {
		return
	}
	select {
	case t.diffQueue <- pngBytes:
	default:
		select {
		case <-t.diffQueue:
		default:
		}
		select {
		case t.diffQueue <- pngBytes:
		default:
		}
	}
}

func (t *Tracker) UpdateDiffImage(pngBytes []byte) error {
	img, err := png.Decode(bytes.NewReader(pngBytes))
	if err != nil {
		return err
	}
	bounds := img.Bounds()
	log.Printf("activity diff image size: %dx%d", bounds.Dx(), bounds.Dy())
	baseAbsX := t.cfg.TopLeftTileX*utils.WplaceTileSize + t.cfg.TopLeftPixelX
	baseAbsY := t.cfg.TopLeftTileY*utils.WplaceTileSize + t.cfg.TopLeftPixelY

	newDiff := make(map[string]Pixel)
	nonZero := 0
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			_, _, _, a := img.At(x, y).RGBA()
			if a == 0 {
				continue
			}
			nonZero++
			absX := baseAbsX + (x - bounds.Min.X)
			absY := baseAbsY + (y - bounds.Min.Y)
			key := pixelKey(absX, absY)
			newDiff[key] = Pixel{AbsX: absX, AbsY: absY}
		}
	}
	log.Printf("activity diff pixels detected: %d", nonZero)

	t.mu.Lock()
	oldDiff := t.currentDiff
	t.currentDiff = newDiff
	for key := range t.vandalState.PixelToPainter {
		if _, ok := newDiff[key]; !ok {
			delete(t.vandalState.PixelToPainter, key)
		}
	}
	t.vandalState.VandalizedPixels = diffPixelsToList(newDiff)
	if err := t.saveVandalStateLocked(); err != nil {
		log.Printf("failed to save vandal state: %v", err)
	}
	t.mu.Unlock()

	changes := make([]Pixel, 0)
	for key, px := range newDiff {
		if _, ok := oldDiff[key]; !ok {
			changes = append(changes, px)
		}
	}
	for key, px := range oldDiff {
		if _, ok := newDiff[key]; !ok {
			changes = append(changes, px)
		}
	}
	if len(changes) == 0 {
		log.Printf("activity diff changes: none")
	}

	for _, px := range changes {
		select {
		case t.queue <- px:
		default:
			log.Printf("activity queue full; dropping pixel %d,%d", px.AbsX, px.AbsY)
		}
	}

	return nil
}

func (t *Tracker) worker() {
	for {
		select {
		case <-t.ctx.Done():
			return
		case px := <-t.queue:
			t.processPixel(px)
		}
	}
}

func (t *Tracker) diffWorker() {
	for {
		select {
		case <-t.ctx.Done():
			return
		case diff := <-t.diffQueue:
			if err := t.UpdateDiffImage(diff); err != nil {
				log.Printf("activity diff update failed: %v", err)
			}
		}
	}
}

func (t *Tracker) processPixel(px Pixel) {
	key := pixelKey(px.AbsX, px.AbsY)

	t.mu.Lock()
	_, isDiff := t.currentDiff[key]
	t.mu.Unlock()
	log.Printf("activity process pixel: %s (isDiff=%t)", key, isDiff)

	painter, err := t.fetchPainter(px)
	if err != nil {
		log.Printf("activity fetch error for %s: %v", key, err)
		return
	}
	if painter == nil {
		log.Printf("activity fetch painter: nil for %s", key)
		return
	}

	now := time.Now().UTC()
	jst := time.FixedZone("JST", 9*3600)
	dateKey := now.In(jst).Format("2006-01-02")

	t.mu.Lock()
	painterID := strconv.Itoa(painter.ID)
	entry := t.activity[painterID]
	if entry == nil {
		entry = &UserActivity{
			ID:            painterID,
			Name:          painter.Name,
			AllianceName:  painter.AllianceName,
		}
		ensureActivityMaps(entry)
		t.activity[painterID] = entry
	} else {
		ensureActivityMaps(entry)
		if painter.Name != "" {
			entry.Name = painter.Name
		}
		if painter.AllianceName != "" {
			entry.AllianceName = painter.AllianceName
		}
	}

	entry.LastSeen = now.Format(time.RFC3339Nano)
	entry.LastPixel = &PixelRef{X: px.AbsX, Y: px.AbsY}

	notifyKind := ""
	shouldNotify := false
	if isDiff {
		entry.VandalCount++
		entry.DailyVandalCounts[dateKey]++
		entry.ActivityScore--
		entry.DailyActivityScores[dateKey]--
		t.vandalState.PixelToPainter[key] = painterID
		if entry.VandalCount == 5 {
			notifyKind = "vandal"
			shouldNotify = true
		}
	} else {
		entry.RestoredCount++
		entry.DailyRestoredCounts[dateKey]++
		entry.ActivityScore++
		entry.DailyActivityScores[dateKey]++
		delete(t.vandalState.PixelToPainter, key)
		if entry.RestoredCount == 5 {
			notifyKind = "fix"
			shouldNotify = true
		}
	}

	if err := t.saveActivityLocked(); err != nil {
		log.Printf("failed to save user activity: %v", err)
	}
	if err := t.saveVandalStateLocked(); err != nil {
		log.Printf("failed to save vandal state: %v", err)
	}
	cb := t.newUserCB
	var userCopy UserActivity
	if shouldNotify {
		userCopy = *entry
	}
	t.mu.Unlock()

	if shouldNotify && cb != nil {
		cb(notifyKind, userCopy)
	}
}

func (t *Tracker) fetchPainter(px Pixel) (*PaintedBy, error) {
	if err := t.waitForBackoff(); err != nil {
		return nil, err
	}
	tileX := px.AbsX / utils.WplaceTileSize
	tileY := px.AbsY / utils.WplaceTileSize
	pixelX := px.AbsX % utils.WplaceTileSize
	pixelY := px.AbsY % utils.WplaceTileSize
	url := fmt.Sprintf("https://backend.wplace.live/s0/pixel/%d/%d?x=%d&y=%d", tileX, tileY, pixelX, pixelY)

	val, err := t.limiter.Do(t.ctx, "backend.wplace.live", func() (interface{}, error) {
		req, err := http.NewRequestWithContext(t.ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		req.Close = true
		req.Header.Set("Connection", "close")
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
		req.Header.Set("Accept", "application/json, text/plain, */*")
		req.Header.Set("Accept-Language", "en-US,en;q=0.9")
		req.Header.Set("Accept-Encoding", "identity")
		req.Header.Set("Sec-Fetch-Dest", "empty")
		req.Header.Set("Sec-Fetch-Mode", "cors")
		req.Header.Set("Sec-Fetch-Site", "same-site")
		req.Header.Set("Sec-CH-UA", "\"Chromium\";v=\"120\", \"Not=A?Brand\";v=\"24\", \"Google Chrome\";v=\"120\"")
		req.Header.Set("Sec-CH-UA-Mobile", "?0")
		req.Header.Set("Sec-CH-UA-Platform", "\"Windows\"")
		resp, err := t.httpClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			body, _ := readResponseBody(resp)
			if resp.StatusCode == http.StatusTooManyRequests {
				t.markBackoff()
			}
			return nil, fmt.Errorf("pixel api status: %s body=%s", resp.Status, string(body))
		}
		t.resetBackoff()
		return readResponseBody(resp)
	})
	if err != nil {
		return nil, err
	}

	var parsed PixelAPIResponse
	if err := json.Unmarshal(val.([]byte), &parsed); err != nil {
		return nil, err
	}
	if parsed.PaintedBy == nil || parsed.PaintedBy.ID == 0 {
		return nil, nil
	}
	return parsed.PaintedBy, nil
}

func (t *Tracker) waitForBackoff() error {
	t.mu.Lock()
	until := t.backoffUntil
	t.mu.Unlock()
	if until.IsZero() {
		return nil
	}
	delay := time.Until(until)
	if delay <= 0 {
		return nil
	}
	select {
	case <-t.ctx.Done():
		return t.ctx.Err()
	case <-time.After(delay):
		return nil
	}
}

func (t *Tracker) markBackoff() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.backoffDelay <= 0 {
		t.backoffDelay = 2 * time.Second
	}
	if t.backoffDelay > 30*time.Second {
		t.backoffDelay = 30 * time.Second
	}
	t.backoffUntil = time.Now().Add(t.backoffDelay)
	t.backoffDelay *= 2
}

func (t *Tracker) resetBackoff() {
	t.mu.Lock()
	t.backoffDelay = 2 * time.Second
	t.backoffUntil = time.Time{}
	t.mu.Unlock()
}

func readResponseBody(resp *http.Response) ([]byte, error) {
	switch resp.Header.Get("Content-Encoding") {
	case "gzip":
		reader, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, err
		}
		defer reader.Close()
		return io.ReadAll(reader)
	case "deflate":
		reader, err := zlib.NewReader(resp.Body)
		if err != nil {
			return nil, err
		}
		defer reader.Close()
		return io.ReadAll(reader)
	default:
		return io.ReadAll(resp.Body)
	}
}

func (t *Tracker) loadState() {
	activityPath := filepath.Join(t.dataDir, "user_activity.json")
	if data, err := os.ReadFile(activityPath); err == nil {
		var entries map[string]*UserActivity
		if err := json.Unmarshal(data, &entries); err == nil {
			dirty := false
			for _, entry := range entries {
				expectedScore := entry.RestoredCount - entry.VandalCount
				if entry.DailyActivityScores == nil || entry.ActivityScore != expectedScore {
					dirty = true
				}
				ensureActivityMaps(entry)
				entry.ActivityScore = expectedScore
				entry.DailyActivityScores = buildDailyActivityScores(entry)
			}
			t.activity = entries
			if dirty {
				if err := t.saveActivityLocked(); err != nil {
					log.Printf("failed to migrate user activity: %v", err)
				}
			}
		}
	}

	vandalPath := filepath.Join(t.dataDir, "vandalized_pixels.json")
	if data, err := os.ReadFile(vandalPath); err == nil {
		var state VandalState
		if err := json.Unmarshal(data, &state); err == nil {
			if state.PixelToPainter == nil {
				state.PixelToPainter = make(map[string]string)
			}
			t.vandalState = state
		}
	}
}

func (t *Tracker) saveActivityLocked() error {
	path := filepath.Join(t.dataDir, "user_activity.json")
	payload, err := json.MarshalIndent(t.activity, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, payload, 0644)
}

func (t *Tracker) saveVandalStateLocked() error {
	path := filepath.Join(t.dataDir, "vandalized_pixels.json")
	payload, err := json.MarshalIndent(t.vandalState, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, payload, 0644)
}

func ensureActivityMaps(entry *UserActivity) {
	if entry.DailyVandalCounts == nil {
		entry.DailyVandalCounts = make(map[string]int)
	}
	if entry.DailyRestoredCounts == nil {
		entry.DailyRestoredCounts = make(map[string]int)
	}
	if entry.DailyActivityScores == nil {
		entry.DailyActivityScores = make(map[string]int)
	}
}

func buildDailyActivityScores(entry *UserActivity) map[string]int {
	out := make(map[string]int)
	for dateKey, count := range entry.DailyVandalCounts {
		out[dateKey] -= count
	}
	for dateKey, count := range entry.DailyRestoredCounts {
		out[dateKey] += count
	}
	return out
}

func diffPixelsToList(diff map[string]Pixel) [][]int {
	out := make([][]int, 0, len(diff))
	keys := make([]string, 0, len(diff))
	for key := range diff {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		px := diff[key]
		out = append(out, []int{px.AbsX, px.AbsY})
	}
	return out
}

func pixelKey(x, y int) string {
	return fmt.Sprintf("(%d, %d)", x, y)
}
