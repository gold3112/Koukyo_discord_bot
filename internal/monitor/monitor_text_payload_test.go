package monitor

import "testing"

func TestHandleTextMessageUpdatePayload(t *testing.T) {
	t.Parallel()

	m := &Monitor{State: NewMonitorState()}
	msg := []byte(`{
		"type":"update",
		"diff_percentage":12.5,
		"diff_pixels":42,
		"weighted_diff_percentage":9.9,
		"chrysanthemum_diff_pixels":11,
		"background_diff_pixels":31,
		"chrysanthemum_total_pixels":100,
		"background_total_pixels":200,
		"total_pixels":300
	}`)

	if err := m.handleTextMessage(msg); err != nil {
		t.Fatalf("handleTextMessage returned error: %v", err)
	}

	data := m.State.GetLatestData()
	if data == nil {
		t.Fatal("expected latest data to be updated")
	}
	if data.DiffPercentage != 12.5 {
		t.Fatalf("DiffPercentage mismatch: got %.2f", data.DiffPercentage)
	}
	if data.DiffPixels != 42 {
		t.Fatalf("DiffPixels mismatch: got %d", data.DiffPixels)
	}
	if data.WeightedDiffPercentage == nil || *data.WeightedDiffPercentage != 9.9 {
		t.Fatalf("WeightedDiffPercentage mismatch: got %v", data.WeightedDiffPercentage)
	}
	if data.ChrysanthemumDiffPixels != 11 || data.BackgroundDiffPixels != 31 {
		t.Fatalf("diff pixel split mismatch: %+v", data)
	}
	if data.TotalPixels != 300 {
		t.Fatalf("TotalPixels mismatch: got %d", data.TotalPixels)
	}
}

func TestHandleTextMessageMetadataPayload(t *testing.T) {
	t.Parallel()

	m := &Monitor{State: NewMonitorState()}
	msg := []byte(`{"type":"metadata","total_pixels":12345}`)

	if err := m.handleTextMessage(msg); err != nil {
		t.Fatalf("handleTextMessage returned error: %v", err)
	}

	data := m.State.GetLatestData()
	if data == nil {
		t.Fatal("expected metadata to update latest data")
	}
	if data.Type != "metadata" {
		t.Fatalf("unexpected type: %q", data.Type)
	}
	if data.TotalPixels != 12345 {
		t.Fatalf("TotalPixels mismatch: got %d", data.TotalPixels)
	}
}

func TestHandleTextMessageErrorPayload(t *testing.T) {
	t.Parallel()

	m := &Monitor{State: NewMonitorState()}
	msg := []byte(`{"type":"error","message":"server down"}`)

	if err := m.handleTextMessage(msg); err != nil {
		t.Fatalf("handleTextMessage returned error: %v", err)
	}
	if data := m.State.GetLatestData(); data != nil {
		t.Fatalf("error payload should not update data, got: %+v", data)
	}
}
