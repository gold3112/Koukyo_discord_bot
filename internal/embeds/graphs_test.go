package embeds

import (
	"Koukyo_discord_bot/internal/monitor"
	"math"
	"testing"
	"time"
)

func TestFormatGraphTickTimeJST(t *testing.T) {
	t.Parallel()

	utc := time.Date(2026, 2, 20, 0, 15, 0, 0, time.UTC)
	got := formatGraphTickTimeJST(utc)
	if got != "09:15" {
		t.Fatalf("unexpected JST tick label: got=%s want=09:15", got)
	}
}

func TestSanitizeDiffHistoryFiltersAndSorts(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 2, 20, 12, 0, 0, 0, time.UTC)
	input := []monitor.DiffRecord{
		{Timestamp: time.Time{}, Percentage: 99},
		{Timestamp: base.Add(2 * time.Minute), Percentage: 2.0},
		{Timestamp: base.Add(1 * time.Minute), Percentage: 1.0},
		{Timestamp: base.Add(3 * time.Minute), Percentage: math.NaN()},
		{Timestamp: base.Add(4 * time.Minute), Percentage: math.Inf(1)},
	}

	out := sanitizeDiffHistory(input)
	if len(out) != 2 {
		t.Fatalf("expected 2 valid records, got %d", len(out))
	}
	if !out[0].Timestamp.Before(out[1].Timestamp) {
		t.Fatalf("records are not sorted by timestamp")
	}
	if out[0].Percentage != 1.0 || out[1].Percentage != 2.0 {
		t.Fatalf("unexpected percentages: %+v", out)
	}
}

func TestEstimateGraphGapThresholdUsesMedian(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 2, 20, 12, 0, 0, 0, time.UTC)
	history := []monitor.DiffRecord{
		{Timestamp: base, Percentage: 1},
		{Timestamp: base.Add(10 * time.Second), Percentage: 2},
		{Timestamp: base.Add(20 * time.Second), Percentage: 3},
		{Timestamp: base.Add(2 * time.Minute), Percentage: 4},
	}

	threshold := estimateGraphGapThreshold(history)
	if threshold != 50*time.Second {
		t.Fatalf("unexpected threshold: got=%s want=50s", threshold)
	}
}
