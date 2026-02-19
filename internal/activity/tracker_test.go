package activity

import (
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
