package embeds

import (
	"bytes"
	"image"
	"image/draw"
	"image/png"
	"io"
	"log"
)

// CombineImages 2つの画像を横に並べて結合する
func CombineImages(liveImageData, diffImageData []byte) (io.Reader, error) {
	log.Printf("CombineImages called: live=%d bytes, diff=%d bytes", len(liveImageData), len(diffImageData))

	// PNG画像をデコード
	liveImg, err := png.Decode(bytes.NewReader(liveImageData))
	if err != nil {
		log.Printf("Failed to decode live image: %v", err)
		return nil, err
	}
	log.Printf("Live image decoded: %dx%d", liveImg.Bounds().Dx(), liveImg.Bounds().Dy())

	diffImg, err := png.Decode(bytes.NewReader(diffImageData))
	if err != nil {
		log.Printf("Failed to decode diff image: %v", err)
		return nil, err
	}
	log.Printf("Diff image decoded: %dx%d", diffImg.Bounds().Dx(), diffImg.Bounds().Dy())

	// 結合画像のサイズを計算
	liveBounds := liveImg.Bounds()
	diffBounds := diffImg.Bounds()

	width := liveBounds.Dx() + diffBounds.Dx()
	height := liveBounds.Dy()
	if diffBounds.Dy() > height {
		height = diffBounds.Dy()
	}

	log.Printf("Combined image size: %dx%d", width, height)

	// 結合画像を作成
	combined := image.NewRGBA(image.Rect(0, 0, width, height))

	// Live画像を左に配置
	draw.Draw(combined, liveBounds, liveImg, liveBounds.Min, draw.Src)

	// Diff画像を右に配置
	diffOffset := image.Pt(liveBounds.Dx(), 0)
	draw.Draw(combined, diffBounds.Add(diffOffset), diffImg, diffBounds.Min, draw.Src)

	// PNGとしてエンコード
	var buf bytes.Buffer
	err = png.Encode(&buf, combined)
	if err != nil {
		log.Printf("Failed to encode combined image: %v", err)
		return nil, err
	}

	log.Printf("Combined image encoded: %d bytes", buf.Len())
	return &buf, nil
}
