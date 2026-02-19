package notifications

import (
	"testing"
	"time"
)

func TestResetAllSmallDiffMessageTrackingClearsPointers(t *testing.T) {
	t.Parallel()

	nextUpdate := time.Now().Add(5 * time.Second)
	n := &Notifier{
		states: map[string]*NotificationState{
			"guild-1": {
				SmallDiffMessageID:        "msg-1",
				SmallDiffMessageChannelID: "ch-1",
				SmallDiffLastContent:      "old content",
				SmallDiffNextUpdate:       nextUpdate,
			},
		},
	}

	n.resetAllSmallDiffMessageTracking()

	st := n.states["guild-1"]
	if st.SmallDiffMessageID != "" {
		t.Fatalf("SmallDiffMessageID should be cleared, got %q", st.SmallDiffMessageID)
	}
	if st.SmallDiffMessageChannelID != "" {
		t.Fatalf("SmallDiffMessageChannelID should be cleared, got %q", st.SmallDiffMessageChannelID)
	}
	if st.SmallDiffLastContent != "" {
		t.Fatalf("SmallDiffLastContent should be cleared, got %q", st.SmallDiffLastContent)
	}
	if !st.SmallDiffNextUpdate.IsZero() {
		t.Fatalf("SmallDiffNextUpdate should be zero, got %s", st.SmallDiffNextUpdate)
	}
}
