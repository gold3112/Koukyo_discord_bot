package embeds

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
)

// BuildHeatmapPNG グリッド集計からヒートマップ画像を生成
// counts: 長さ=gridW*gridH のカウント
func BuildHeatmapPNG(counts []uint32, gridW, gridH int, outW, outH int) (*bytes.Buffer, error) {
	img := image.NewRGBA(image.Rect(0, 0, outW, outH))
	// 背景
	draw.Draw(img, img.Bounds(), &image.Uniform{C: color.RGBA{0, 0, 0, 255}}, image.Point{}, draw.Src)

	if counts == nil || gridW <= 0 || gridH <= 0 {
		buf := &bytes.Buffer{}
		if err := png.Encode(buf, img); err != nil {
			return nil, err
		}
		return buf, nil
	}

	// 最大値
	var maxv uint32
	for _, v := range counts {
		if v > maxv {
			maxv = v
		}
	}
	if maxv == 0 {
		maxv = 1
	}

	// アスペクト比を維持して中央に配置
	scaleX := float64(outW) / float64(gridW)
	scaleY := float64(outH) / float64(gridH)
	scale := scaleX
	if scaleY < scale {
		scale = scaleY
	}
	targetW := int(float64(gridW) * scale)
	targetH := int(float64(gridH) * scale)
	if targetW < 1 {
		targetW = 1
	}
	if targetH < 1 {
		targetH = 1
	}
	offsetX := (outW - targetW) / 2
	offsetY := (outH - targetH) / 2

	// 着色
	for gy := 0; gy < gridH; gy++ {
		for gx := 0; gx < gridW; gx++ {
			v := counts[gy*gridW+gx]
			if v == 0 {
				continue
			}
			norm := math.Log1p(float64(v)) / math.Log1p(float64(maxv))
			col := heatColor(norm)
			x0 := offsetX + int(float64(gx)*scale)
			y0 := offsetY + int(float64(gy)*scale)
			x1 := offsetX + int(float64(gx+1)*scale)
			y1 := offsetY + int(float64(gy+1)*scale)
			if x1 <= x0 || y1 <= y0 {
				continue
			}
			rect := image.Rect(x0, y0, x1, y1)
			draw.Draw(img, rect, &image.Uniform{C: col}, image.Point{}, draw.Src)
		}
	}

	buf := &bytes.Buffer{}
	if err := png.Encode(buf, img); err != nil {
		return nil, err
	}
	return buf, nil
}

// heatColor 正規化値(0..1)から擬似カラー(黒→赤→黄白)
func heatColor(t float64) color.RGBA {
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	// 黒(0,0,0)→赤(255,0,0)→黄白(255,255,224)
	if t < 0.5 {
		u := t / 0.5
		r := uint8(255 * u)
		return color.RGBA{r, 0, 0, 255}
	}
	u := (t - 0.5) / 0.5
	r := uint8(255)
	g := uint8(255 * u)
	b := uint8(224 * u)
	return color.RGBA{r, g, b, 255}
}
