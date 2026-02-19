package notifications

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
)

func TestSmallDiffCoordinatesFromDiffPNG(t *testing.T) {
	t.Parallel()

	diffPNG := mustEncodeDiff(t, map[[2]int]bool{
		{0, 0}: true,
		{1, 0}: true,
	})

	coords, err := smallDiffCoordinatesFromDiffPNG(diffPNG, 10)
	if err != nil {
		t.Fatalf("smallDiffCoordinatesFromDiffPNG returned error: %v", err)
	}
	if len(coords) != 2 {
		t.Fatalf("expected 2 coords, got %d", len(coords))
	}

	if got := coords[0]; got.TileX != 1818 || got.TileY != 806 || got.PixelX != 989 || got.PixelY != 358 {
		t.Fatalf("unexpected first coord: %+v", got)
	}
	if got := coords[1]; got.TileX != 1818 || got.TileY != 806 || got.PixelX != 990 || got.PixelY != 358 {
		t.Fatalf("unexpected second coord: %+v", got)
	}
}

func TestSmallDiffCoordinateLines(t *testing.T) {
	t.Parallel()

	diffPNG := mustEncodeDiff(t, map[[2]int]bool{
		{0, 0}: true,
	})

	lines, err := smallDiffCoordinateLines(diffPNG, 10)
	if err != nil {
		t.Fatalf("smallDiffCoordinateLines returned error: %v", err)
	}
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}

	if !strings.Contains(lines[0], "(1818-806-989-358:https://wplace.live/?") {
		t.Fatalf("line does not include expected coordinate+url format: %s", lines[0])
	}
	if !strings.Contains(lines[0], "zoom=21.17") {
		t.Fatalf("line does not use high detail zoom: %s", lines[0])
	}
}

func mustEncodeDiff(t *testing.T, pixels map[[2]int]bool) []byte {
	t.Helper()

	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	for pos := range pixels {
		img.Set(pos[0], pos[1], color.NRGBA{R: 255, G: 255, B: 255, A: 255})
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("failed to encode test png: %v", err)
	}
	return buf.Bytes()
}
