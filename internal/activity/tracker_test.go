package activity

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
	"time"
)

func TestCloneUserActivityDeepCopy(t *testing.T) {
	src := &UserActivity{
		ID:                  "1",
		Name:                "user",
		DailyVandalCounts:   map[string]int{"2026-02-15": 3},
		DailyRestoredCounts: map[string]int{"2026-02-15": 1},
		DailyActivityScores: map[string]int{"2026-02-15": -2},
		LastPixel:           &PixelRef{X: 10, Y: 20},
	}

	cloned := cloneUserActivity(src)

	src.DailyVandalCounts["2026-02-15"] = 999
	src.DailyRestoredCounts["2026-02-15"] = 999
	src.DailyActivityScores["2026-02-15"] = 999
	src.LastPixel.X = 777

	if got := cloned.DailyVandalCounts["2026-02-15"]; got != 3 {
		t.Fatalf("DailyVandalCounts not deep-copied, got=%d want=3", got)
	}
	if got := cloned.DailyRestoredCounts["2026-02-15"]; got != 1 {
		t.Fatalf("DailyRestoredCounts not deep-copied, got=%d want=1", got)
	}
	if got := cloned.DailyActivityScores["2026-02-15"]; got != -2 {
		t.Fatalf("DailyActivityScores not deep-copied, got=%d want=-2", got)
	}
	if cloned.LastPixel == nil {
		t.Fatal("LastPixel should not be nil")
	}
	if got := cloned.LastPixel.X; got != 10 {
		t.Fatalf("LastPixel not deep-copied, got=%d want=10", got)
	}
}

func TestPowerSaveInferenceLifecycle(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 2, 20, 12, 0, 0, 0, time.UTC)
	var state powerSaveInferenceState

	if armed := armPowerSaveInference(&state, 1, now); armed {
		t.Fatal("inference should not arm for single pixel")
	}
	if state.Active {
		t.Fatal("state should remain inactive")
	}

	if armed := armPowerSaveInference(&state, 8, now); !armed {
		t.Fatal("expected inference to be armed")
	}
	if !state.Active || state.RemainingPixels != 8 {
		t.Fatalf("unexpected armed state: %+v", state)
	}

	active, effective, credit := beginPowerSaveInference(&state, "1001", now.Add(2*time.Second))
	if !active {
		t.Fatal("inference should be active")
	}
	if effective != "1001" || credit != 8 {
		t.Fatalf("unexpected begin result effective=%s credit=%d", effective, credit)
	}

	consumePowerSaveInference(&state)
	if state.RemainingPixels != 7 {
		t.Fatalf("remaining should decrement to 7, got %d", state.RemainingPixels)
	}

	// Subsequent detections are still attributed to the first claimed painter.
	active, effective, credit = beginPowerSaveInference(&state, "2002", now.Add(3*time.Second))
	if !active || effective != "1001" || credit != 0 {
		t.Fatalf("unexpected aliasing result active=%v effective=%s credit=%d", active, effective, credit)
	}

	state.ExpiresAt = now.Add(-time.Second)
	active, _, _ = beginPowerSaveInference(&state, "1001", now)
	if active {
		t.Fatal("expired inference should not stay active")
	}
	if state.Active {
		t.Fatal("state should be reset after expiration")
	}
}

func TestRecordRecentEventsCapped(t *testing.T) {
	t.Parallel()

	store := map[string][]time.Time{}
	now := time.Now().UTC()
	got := recordRecentEvents(store, "u1", now, newUserNotifyWindow, 999)
	if got != newUserNotifyThreshold {
		t.Fatalf("expected capped recent events count %d, got %d", newUserNotifyThreshold, got)
	}
}

func TestClaimDeferredPixels(t *testing.T) {
	t.Parallel()

	state := powerSaveInferenceState{Active: true}
	deferPowerSaveInferencePixel(&state, Pixel{AbsX: 10, AbsY: 20})
	deferPowerSaveInferencePixel(&state, Pixel{AbsX: 11, AbsY: 21})

	vs := &VandalState{PixelToPainter: map[string]string{}}
	claimed := claimDeferredPixels(&state, "1001", vs)
	if claimed != 2 {
		t.Fatalf("expected claimed deferred pixels=2, got %d", claimed)
	}
	if len(state.DeferredPixels) != 0 {
		t.Fatalf("deferred pixels should be cleared after claim")
	}
	if got := vs.PixelToPainter[pixelKey(10, 20)]; got != "1001" {
		t.Fatalf("unexpected painter for first deferred pixel: %q", got)
	}
	if got := vs.PixelToPainter[pixelKey(11, 21)]; got != "1001" {
		t.Fatalf("unexpected painter for second deferred pixel: %q", got)
	}
}

func TestUpdateDiffImageInferenceQueuesSingleProbe(t *testing.T) {
	t.Parallel()

	tracker := NewTracker(Config{
		TopLeftTileX:  0,
		TopLeftTileY:  0,
		TopLeftPixelX: 0,
		TopLeftPixelY: 0,
		Width:         2,
		Height:        2,
	}, nil, "")

	tracker.ArmPowerSaveResumeInference(3)
	first := mustEncodeDiffPNG(t, map[[2]int]bool{
		{0, 0}: true,
		{1, 0}: true,
		{0, 1}: true,
	})
	if err := tracker.UpdateDiffImage(first); err != nil {
		t.Fatalf("UpdateDiffImage(first) returned error: %v", err)
	}

	if got := len(tracker.queue); got != 1 {
		t.Fatalf("expected queue length 1 (probe only), got %d", got)
	}

	tracker.mu.Lock()
	if !tracker.powerSaveInference.ProbeQueued {
		t.Fatalf("expected ProbeQueued=true after first update")
	}
	if got := len(tracker.powerSaveInference.DeferredPixels); got != 2 {
		t.Fatalf("expected 2 deferred pixels, got %d", got)
	}
	tracker.mu.Unlock()

	second := mustEncodeDiffPNG(t, map[[2]int]bool{
		{0, 0}: true,
		{1, 0}: true,
		{0, 1}: true,
		{1, 1}: true, // +1 added while probe still pending
	})
	if err := tracker.UpdateDiffImage(second); err != nil {
		t.Fatalf("UpdateDiffImage(second) returned error: %v", err)
	}

	if got := len(tracker.queue); got != 1 {
		t.Fatalf("expected still 1 queued probe after second update, got %d", got)
	}
	tracker.mu.Lock()
	if got := len(tracker.powerSaveInference.DeferredPixels); got != 3 {
		t.Fatalf("expected deferred pixels to grow to 3, got %d", got)
	}
	tracker.mu.Unlock()
}

func mustEncodeDiffPNG(t *testing.T, pixels map[[2]int]bool) []byte {
	t.Helper()

	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	for pos := range pixels {
		img.Set(pos[0], pos[1], color.NRGBA{R: 255, G: 255, B: 255, A: 255})
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("failed to encode diff png: %v", err)
	}
	return buf.Bytes()
}
