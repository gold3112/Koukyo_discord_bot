package embeds

import (
	"Koukyo_discord_bot/internal/monitor"
	"bytes"
	"image"
	"image/color/palette"
	"image/gif"
	"image/png"
)

// BuildTimelapseGIF TimelapseFrame列からGIFを生成
func BuildTimelapseGIF(frames []monitor.TimelapseFrame) (*bytes.Buffer, error) {
	out := &gif.GIF{}
	delay := 7 // 0.07s/frame (単位: 1/100秒)
	for _, f := range frames {
		img, err := pngToPaletted(f.DiffPNG)
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

func pngToPaletted(pngBytes []byte) (*image.Paletted, error) {
	img, err := png.Decode(bytes.NewReader(pngBytes))
	if err != nil {
		return nil, err
	}
	bounds := img.Bounds()
	paletted := imageToPaletted(img, bounds)
	return paletted, nil
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
