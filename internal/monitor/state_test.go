package monitor

import (
	"container/ring"
	"testing"
	"time"
)

func TestGetDiffHistorySkipsZeroTimestamp(t *testing.T) {
	t.Parallel()

	ms := NewMonitorState()
	ms.DiffHistory = ring.New(4)

	now := time.Now()
	r := ms.DiffHistory
	r.Value = DiffRecord{Timestamp: time.Time{}, Percentage: 99}
	r = r.Next()
	r.Value = DiffRecord{Timestamp: now.Add(-2 * time.Hour), Percentage: 1.0}
	r = r.Next()
	r.Value = DiffRecord{Timestamp: now.Add(-2 * time.Minute), Percentage: 2.0}

	recent := ms.GetDiffHistory(10*time.Minute, false)
	if len(recent) != 1 {
		t.Fatalf("expected 1 recent record, got %d", len(recent))
	}
	if recent[0].Timestamp.IsZero() {
		t.Fatalf("recent record must not include zero timestamp")
	}
	if recent[0].Percentage != 2.0 {
		t.Fatalf("unexpected recent percentage: %.2f", recent[0].Percentage)
	}

	all := ms.GetDiffHistory(0, false)
	if len(all) != 2 {
		t.Fatalf("expected 2 non-zero records, got %d", len(all))
	}
	for _, rec := range all {
		if rec.Timestamp.IsZero() {
			t.Fatalf("all records must not include zero timestamp")
		}
	}
}
