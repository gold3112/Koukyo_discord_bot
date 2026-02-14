package activity

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image/png"
	"log"
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
	AllianceID   int    `json:"allianceId"`
	AllianceName string `json:"allianceName"`
	EquippedFlag int    `json:"equippedFlag"`
	Picture      string `json:"picture"`
	Discord      string `json:"discord"`
	DiscordID    string `json:"discordId"`
}

type PixelRef struct {
	X int `json:"x"`
	Y int `json:"y"`
}

type UserActivity struct {
	ID                  string         `json:"id"`
	Name                string         `json:"name"`
	AllianceName        string         `json:"allianceName"`
	Discord             string         `json:"discord,omitempty"`
	DiscordID           string         `json:"discord_id,omitempty"`
	Picture             string         `json:"picture,omitempty"`
	LastSeen            string         `json:"last_seen"`
	VandalCount         int            `json:"vandal_count"`
	RestoredCount       int            `json:"restored_count"`
	ActivityScore       int            `json:"activity_score"`
	DailyVandalCounts   map[string]int `json:"daily_vandal_counts,omitempty"`
	DailyRestoredCounts map[string]int `json:"daily_restored_counts,omitempty"`
	DailyActivityScores map[string]int `json:"daily_activity_scores,omitempty"`
	LastPixel           *PixelRef      `json:"last_pixel,omitempty"`
	VandalNotified      bool           `json:"vandal_notified,omitempty"`
	FixNotified         bool           `json:"fix_notified,omitempty"`
}

type VandalState struct {
	VandalizedPixels [][]int           `json:"vandalized_pixels"`
	PixelToPainter   map[string]string `json:"pixel_to_painter"`
}

type DailyPixelCounts struct {
	Vandal map[string]int `json:"vandal"`
	Fix    map[string]int `json:"fix"`
}

const (
	newUserNotifyThreshold = 5
	newUserNotifyWindow    = 5 * time.Minute
	stateFlushInterval     = 2 * time.Second
	recentEventsGCInterval = 1 * time.Minute
)

type Tracker struct {
	cfg          Config
	limiter      *utils.RateLimiter
	dataDir      string
	httpClient   *http.Client
	newUserCB    NewUserCallback
	queue        chan Pixel
	pending      map[string]Pixel
	diffQueue    chan []byte
	ctx          context.Context
	cancel       context.CancelFunc
	mu           sync.Mutex
	currentDiff  map[string]Pixel
	activity     map[string]*UserActivity
	vandalState  VandalState
	dailyCounts  DailyPixelCounts
	backoffDelay time.Duration
	backoffUntil time.Time
	// Recent event windows for "N actions within 5 minutes" detection.
	recentVandalEvents map[string][]time.Time
	recentFixEvents    map[string][]time.Time
	dirtyActivity      bool
	dirtyVandalState   bool
	dirtyDailyCounts   bool
}

type NewUserCallback func(kind string, user UserActivity)

func NewTracker(cfg Config, limiter *utils.RateLimiter, dataDir string) *Tracker {
	ctx, cancel := context.WithCancel(context.Background())
	queueSize := cfg.Width * cfg.Height
	if queueSize <= 0 {
		queueSize = 4096
	}
	diffQueueSize := 1
	t := &Tracker{
		cfg:                cfg,
		limiter:            limiter,
		dataDir:            dataDir,
		httpClient:         NewPixelHTTPClient(),
		queue:              make(chan Pixel, queueSize),
		pending:            make(map[string]Pixel),
		diffQueue:          make(chan []byte, diffQueueSize),
		ctx:                ctx,
		cancel:             cancel,
		currentDiff:        make(map[string]Pixel),
		activity:           make(map[string]*UserActivity),
		vandalState:        VandalState{PixelToPainter: make(map[string]string)},
		backoffDelay:       2 * time.Second,
		recentVandalEvents: make(map[string][]time.Time),
		recentFixEvents:    make(map[string][]time.Time),
	}
	t.loadState()
	t.loadDailyCounts()
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
	go t.flushWorker()
	go t.recentEventsGCWorker()
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
	added, removed := countDiffChanges(oldDiff, newDiff)
	dateKey := dateKeyJST()
	if t.dailyCounts.Vandal == nil {
		t.dailyCounts.Vandal = make(map[string]int)
	}
	if t.dailyCounts.Fix == nil {
		t.dailyCounts.Fix = make(map[string]int)
	}
	t.dailyCounts.Vandal[dateKey] += added
	t.dailyCounts.Fix[dateKey] += removed
	t.dirtyVandalState = true
	t.dirtyDailyCounts = true
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
		t.enqueuePixel(px)
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
			ID:           painterID,
			Name:         painter.Name,
			AllianceName: painter.AllianceName,
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
	if painter.Discord != "" {
		entry.Discord = painter.Discord
	}
	if painter.DiscordID != "" {
		entry.DiscordID = painter.DiscordID
	}
	if painter.Picture != "" {
		entry.Picture = painter.Picture
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
		windowCount := recordRecentEvent(t.recentVandalEvents, painterID, now, newUserNotifyWindow)
		if !entry.VandalNotified && windowCount >= newUserNotifyThreshold {
			notifyKind = "vandal"
			shouldNotify = true
			entry.VandalNotified = true
		}
	} else {
		entry.RestoredCount++
		entry.DailyRestoredCounts[dateKey]++
		entry.ActivityScore++
		entry.DailyActivityScores[dateKey]++
		delete(t.vandalState.PixelToPainter, key)
		windowCount := recordRecentEvent(t.recentFixEvents, painterID, now, newUserNotifyWindow)
		if !entry.FixNotified && windowCount >= newUserNotifyThreshold {
			notifyKind = "fix"
			shouldNotify = true
			entry.FixNotified = true
		}
	}

	t.dirtyActivity = true
	t.dirtyVandalState = true
	cb := t.newUserCB
	var userCopy UserActivity
	if shouldNotify {
		userCopy = *entry
	}
	t.mu.Unlock()

	if shouldNotify && cb != nil {
		cb(notifyKind, userCopy)
	}

	t.mu.Lock()
	delete(t.pending, key)
	t.mu.Unlock()
}

func (t *Tracker) fetchPainter(px Pixel) (*PaintedBy, error) {
	if err := t.waitForBackoff(); err != nil {
		return nil, err
	}
	tileX := px.AbsX / utils.WplaceTileSize
	tileY := px.AbsY / utils.WplaceTileSize
	pixelX := px.AbsX % utils.WplaceTileSize
	pixelY := px.AbsY % utils.WplaceTileSize
	parsed, status, err := FetchPixelInfo(t.ctx, t.httpClient, t.limiter, tileX, tileY, pixelX, pixelY)
	if err != nil {
		if status == http.StatusTooManyRequests {
			t.markBackoff()
		}
		return nil, err
	}
	t.resetBackoff()
	if parsed == nil || parsed.PaintedBy == nil || parsed.PaintedBy.ID == 0 {
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
				if entry.VandalCount >= newUserNotifyThreshold && !entry.VandalNotified {
					entry.VandalNotified = true
					dirty = true
				}
				if entry.RestoredCount >= newUserNotifyThreshold && !entry.FixNotified {
					entry.FixNotified = true
					dirty = true
				}
			}
			t.activity = entries
			if dirty {
				if err := t.saveActivitySnapshot(); err != nil {
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

func (t *Tracker) loadDailyCounts() {
	path := filepath.Join(t.dataDir, "vandal_daily.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var counts DailyPixelCounts
	if err := json.Unmarshal(data, &counts); err != nil {
		return
	}
	if counts.Vandal == nil {
		counts.Vandal = make(map[string]int)
	}
	if counts.Fix == nil {
		counts.Fix = make(map[string]int)
	}
	t.dailyCounts = counts
}

func (t *Tracker) saveActivitySnapshot() error {
	t.mu.Lock()
	payload, err := json.MarshalIndent(t.activity, "", "  ")
	t.mu.Unlock()
	if err != nil {
		return err
	}
	return t.writeFileAtomic("user_activity.json", payload)
}

func (t *Tracker) flushWorker() {
	ticker := time.NewTicker(stateFlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-t.ctx.Done():
			t.flushDirtyState()
			return
		case <-ticker.C:
			t.flushDirtyState()
		}
	}
}

func (t *Tracker) flushDirtyState() {
	type filePayload struct {
		name string
		data []byte
	}
	payloads := make([]filePayload, 0, 3)

	t.mu.Lock()
	flushActivity := t.dirtyActivity
	flushVandal := t.dirtyVandalState
	flushDaily := t.dirtyDailyCounts

	if flushActivity {
		if data, err := json.MarshalIndent(t.activity, "", "  "); err != nil {
			log.Printf("failed to marshal user activity: %v", err)
		} else {
			payloads = append(payloads, filePayload{name: "user_activity.json", data: data})
			t.dirtyActivity = false
		}
	}
	if flushVandal {
		if data, err := json.MarshalIndent(t.vandalState, "", "  "); err != nil {
			log.Printf("failed to marshal vandal state: %v", err)
		} else {
			payloads = append(payloads, filePayload{name: "vandalized_pixels.json", data: data})
			t.dirtyVandalState = false
		}
	}
	if flushDaily {
		if data, err := json.MarshalIndent(t.dailyCounts, "", "  "); err != nil {
			log.Printf("failed to marshal daily counts: %v", err)
		} else {
			payloads = append(payloads, filePayload{name: "vandal_daily.json", data: data})
			t.dirtyDailyCounts = false
		}
	}
	t.mu.Unlock()

	for _, p := range payloads {
		if err := t.writeFileAtomic(p.name, p.data); err != nil {
			log.Printf("failed to save %s: %v", p.name, err)
		}
	}
}

func (t *Tracker) recentEventsGCWorker() {
	ticker := time.NewTicker(recentEventsGCInterval)
	defer ticker.Stop()

	for {
		select {
		case <-t.ctx.Done():
			return
		case <-ticker.C:
			t.cleanupRecentEvents(time.Now().UTC())
		}
	}
}

func (t *Tracker) cleanupRecentEvents(now time.Time) {
	cutoff := now.Add(-newUserNotifyWindow)
	t.mu.Lock()
	pruneRecentEventStore(t.recentVandalEvents, cutoff)
	pruneRecentEventStore(t.recentFixEvents, cutoff)
	t.mu.Unlock()
}

func (t *Tracker) writeFileAtomic(filename string, payload []byte) error {
	if t.dataDir == "" {
		return fmt.Errorf("dataDir is empty")
	}
	if err := os.MkdirAll(t.dataDir, 0755); err != nil {
		return err
	}
	path := filepath.Join(t.dataDir, filename)
	tmp, err := os.CreateTemp(t.dataDir, filename+".tmp.*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(payload); err != nil {
		tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		if removeErr := os.Remove(path); removeErr != nil && !os.IsNotExist(removeErr) {
			_ = os.Remove(tmpName)
			return removeErr
		}
		if err := os.Rename(tmpName, path); err != nil {
			_ = os.Remove(tmpName)
			return err
		}
	}
	return nil
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

func (t *Tracker) enqueuePixel(px Pixel) {
	key := pixelKey(px.AbsX, px.AbsY)
	t.mu.Lock()
	if _, exists := t.pending[key]; exists {
		t.mu.Unlock()
		return
	}
	t.pending[key] = px
	t.mu.Unlock()

	select {
	case <-t.ctx.Done():
		t.mu.Lock()
		delete(t.pending, key)
		t.mu.Unlock()
	case t.queue <- px:
	}
}

func countDiffChanges(oldDiff, newDiff map[string]Pixel) (added, removed int) {
	for key := range newDiff {
		if _, ok := oldDiff[key]; !ok {
			added++
		}
	}
	for key := range oldDiff {
		if _, ok := newDiff[key]; !ok {
			removed++
		}
	}
	return added, removed
}

func dateKeyJST() string {
	jst := time.FixedZone("JST", 9*3600)
	return time.Now().In(jst).Format("2006-01-02")
}

func pixelKey(x, y int) string {
	return fmt.Sprintf("(%d, %d)", x, y)
}

func recordRecentEvent(
	store map[string][]time.Time,
	userID string,
	now time.Time,
	window time.Duration,
) int {
	events := append(store[userID], now)
	cutoff := now.Add(-window)
	write := 0
	for _, ts := range events {
		if ts.Before(cutoff) {
			continue
		}
		events[write] = ts
		write++
	}
	events = events[:write]
	store[userID] = events
	return len(events)
}

func pruneRecentEventStore(store map[string][]time.Time, cutoff time.Time) {
	for userID, events := range store {
		write := 0
		for _, ts := range events {
			if ts.Before(cutoff) {
				continue
			}
			events[write] = ts
			write++
		}
		if write == 0 {
			delete(store, userID)
			continue
		}
		store[userID] = events[:write]
	}
}
