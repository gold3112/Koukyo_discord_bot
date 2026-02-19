package commands

import (
	"Koukyo_discord_bot/internal/monitor"
	"math"
	"testing"
	"time"
)

func TestEstimateRepairRatePerSecondDecreasing(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 2, 20, 12, 0, 0, 0, time.UTC)
	history := []monitor.DiffRecord{
		{Timestamp: base, Percentage: 10.0},
		{Timestamp: base.Add(60 * time.Second), Percentage: 9.0},
		{Timestamp: base.Add(120 * time.Second), Percentage: 8.0},
	}

	rate, method, ok := estimateRepairRatePerSecond(history)
	if !ok {
		t.Fatalf("expected repair rate to be estimable")
	}
	if method != "線形回帰" {
		t.Fatalf("unexpected method: %s", method)
	}
	if math.Abs(rate-(1.0/60.0)) > 1e-6 {
		t.Fatalf("unexpected rate: %.8f", rate)
	}
}

func TestEstimateRepairRatePerSecondIncreasing(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 2, 20, 12, 0, 0, 0, time.UTC)
	history := []monitor.DiffRecord{
		{Timestamp: base, Percentage: 1.0},
		{Timestamp: base.Add(60 * time.Second), Percentage: 2.0},
		{Timestamp: base.Add(120 * time.Second), Percentage: 3.0},
	}

	_, _, ok := estimateRepairRatePerSecond(history)
	if ok {
		t.Fatalf("expected no repair estimate for increasing trend")
	}
}

func TestSanitizePredictHistory(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 2, 20, 12, 0, 0, 0, time.UTC)
	history := []monitor.DiffRecord{
		{Timestamp: base.Add(2 * time.Minute), Percentage: 2.0},
		{Timestamp: time.Time{}, Percentage: 999},
		{Timestamp: base.Add(1 * time.Minute), Percentage: 1.0},
		{Timestamp: base.Add(3 * time.Minute), Percentage: math.NaN()},
	}

	out := sanitizePredictHistory(history)
	if len(out) != 2 {
		t.Fatalf("expected 2 valid records, got %d", len(out))
	}
	if !out[0].Timestamp.Before(out[1].Timestamp) {
		t.Fatalf("expected sorted timestamps")
	}
}

func TestFormatPredictETA(t *testing.T) {
	t.Parallel()

	got := formatPredictETA(26*time.Hour + 3*time.Minute + 4*time.Second)
	if got != "1日 2時間 3分" {
		t.Fatalf("unexpected formatted eta: %s", got)
	}
}
