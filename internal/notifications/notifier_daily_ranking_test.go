package notifications

import (
	"io"
	"testing"
)

func TestBuildPeakFilesForSendCreatesFreshReader(t *testing.T) {
	t.Parallel()

	attachmentData := []byte{1, 2, 3, 4}
	filesA := buildPeakFilesForSend(attachmentData, "daily_peak_diff.png")
	if len(filesA) != 1 {
		t.Fatalf("expected 1 file, got %d", len(filesA))
	}
	readA, err := io.ReadAll(filesA[0].Reader)
	if err != nil {
		t.Fatalf("failed to read first reader: %v", err)
	}
	if len(readA) != len(attachmentData) {
		t.Fatalf("first reader length mismatch: got %d want %d", len(readA), len(attachmentData))
	}

	filesB := buildPeakFilesForSend(attachmentData, "daily_peak_diff.png")
	if len(filesB) != 1 {
		t.Fatalf("expected 1 file on second build, got %d", len(filesB))
	}
	readB, err := io.ReadAll(filesB[0].Reader)
	if err != nil {
		t.Fatalf("failed to read second reader: %v", err)
	}
	if len(readB) != len(attachmentData) {
		t.Fatalf("second reader length mismatch: got %d want %d", len(readB), len(attachmentData))
	}
}

func TestBuildPeakImageAttachmentDataFallsBackToDiffCopy(t *testing.T) {
	t.Parallel()

	diff := []byte{9, 8, 7}
	gotData, gotName := buildPeakImageAttachmentData([]byte("invalid png"), diff, true)
	if gotName != "daily_peak_diff.png" {
		t.Fatalf("unexpected attachment name: %s", gotName)
	}
	if len(gotData) != len(diff) {
		t.Fatalf("unexpected attachment length: got %d want %d", len(gotData), len(diff))
	}

	// Ensure returned data is a copy, not aliasing input slice.
	gotData[0] = 0
	if diff[0] != 9 {
		t.Fatalf("diff slice was mutated; expected copied data")
	}
}
