package activity

import "testing"

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
