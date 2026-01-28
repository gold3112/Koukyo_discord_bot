package embeds

import (
	"Koukyo_discord_bot/internal/monitor"
	"bytes"
	"errors"
	"image"
	"image/color/palette"
	"image/draw"
	"image/gif"
	"image/png"
)

// BuildTimelapseGIF TimelapseFrame列からGIFを生成
func BuildTimelapseGIF(frames []monitor.TimelapseFrame) (*bytes.Buffer, error) {
	if len(frames) == 0 {
		return nil, errors.New("timelapse: no frames")
	}
	out := &gif.GIF{}
	delay := 7 // 0.07s/frame (単位: 1/100秒)
	for _, f := range frames {
		img, err := frameToPaletted(f)
		if err != nil {
			return nil, err
		}
		out.Image = append(out.Image, img)
		out.Delay = append(out.Delay, delay)
	}
	buf := &bytes.Buffer{}
	if err := gif.EncodeAll(buf, out); err != nil {
		return nil, err
	}
	return buf, nil
}

func frameToPaletted(frame monitor.TimelapseFrame) (*image.Paletted, error) {
	if len(frame.LivePNG) > 0 && len(frame.DiffPNG) > 0 {
		return combineToPaletted(frame.LivePNG, frame.DiffPNG)
	}
	if len(frame.DiffPNG) > 0 {
		return pngToPaletted(frame.DiffPNG)
	}
	if len(frame.LivePNG) > 0 {
		return pngToPaletted(frame.LivePNG)
	}
	return nil, errors.New("timelapse: empty frame")
}

func pngToPaletted(pngBytes []byte) (*image.Paletted, error) {
	img, err := png.Decode(bytes.NewReader(pngBytes))
	if err != nil {
		return nil, err
	}
	bounds := img.Bounds()
	paletted := imageToPaletted(img, bounds)
	return paletted, nil
}

func combineToPaletted(livePNG, diffPNG []byte) (*image.Paletted, error) {
	liveImg, err := png.Decode(bytes.NewReader(livePNG))
	if err != nil {
		return nil, err
	}
	diffImg, err := png.Decode(bytes.NewReader(diffPNG))
	if err != nil {
		return nil, err
	}
	liveBounds := liveImg.Bounds()
	diffBounds := diffImg.Bounds()

	width := liveBounds.Dx() + diffBounds.Dx()
	height := liveBounds.Dy()
	if diffBounds.Dy() > height {
		height = diffBounds.Dy()
	}

	combined := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(combined, liveBounds, liveImg, liveBounds.Min, draw.Src)
	diffOffset := image.Pt(liveBounds.Dx(), 0)
	draw.Draw(combined, diffBounds.Add(diffOffset), diffImg, diffBounds.Min, draw.Src)

	return imageToPaletted(combined, combined.Bounds()), nil
}

func imageToPaletted(src image.Image, bounds image.Rectangle) *image.Paletted {
	p := palette.Plan9
	dst := image.NewPaletted(bounds, p)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			dst.Set(x, y, src.At(x, y))
		}
	}
	return dst
}
