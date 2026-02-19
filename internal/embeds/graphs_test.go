package embeds

import (
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
