package embeds

import (
	"Koukyo_discord_bot/internal/monitor"
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"testing"
)

func TestBuildTimelapseGIFLastFrameHeldForOneSecond(t *testing.T) {
	t.Parallel()

	frames := []monitor.TimelapseFrame{
		{DiffPNG: mustEncodeSolidPNG(t, color.NRGBA{R: 255, A: 255})},
		{DiffPNG: mustEncodeSolidPNG(t, color.NRGBA{G: 255, A: 255})},
	}

	buf, err := BuildTimelapseGIF(frames)
	if err != nil {
		t.Fatalf("BuildTimelapseGIF returned error: %v", err)
	}

	decoded, err := gif.DecodeAll(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("failed to decode generated gif: %v", err)
	}
	if len(decoded.Delay) != 2 {
		t.Fatalf("expected 2 delays, got %d", len(decoded.Delay))
	}
	if decoded.Delay[0] != 7 {
		t.Fatalf("unexpected first frame delay: got=%d want=7", decoded.Delay[0])
	}
	if decoded.Delay[1] != 100 {
		t.Fatalf("unexpected final frame delay: got=%d want=100", decoded.Delay[1])
	}
}

func TestBuildTimelapseGIFSingleFrameAlsoHeld(t *testing.T) {
	t.Parallel()

	frames := []monitor.TimelapseFrame{
		{DiffPNG: mustEncodeSolidPNG(t, color.NRGBA{B: 255, A: 255})},
	}

	buf, err := BuildTimelapseGIF(frames)
	if err != nil {
		t.Fatalf("BuildTimelapseGIF returned error: %v", err)
	}

	decoded, err := gif.DecodeAll(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("failed to decode generated gif: %v", err)
	}
	if len(decoded.Delay) != 1 {
		t.Fatalf("expected 1 delay, got %d", len(decoded.Delay))
	}
	if decoded.Delay[0] != 100 {
		t.Fatalf("unexpected single frame delay: got=%d want=100", decoded.Delay[0])
	}
}

func mustEncodeSolidPNG(t *testing.T, c color.NRGBA) []byte {
	t.Helper()

	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			img.SetNRGBA(x, y, c)
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("failed to encode png: %v", err)
	}
	return buf.Bytes()
}
