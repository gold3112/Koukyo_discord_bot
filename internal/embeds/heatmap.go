package embeds

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/png"
)

// BuildHeatmapPNG グリッド集計からヒートマップ画像を生成
// counts: 長さ=gridW*gridH のカウント
func BuildHeatmapPNG(counts []uint32, gridW, gridH int, outW, outH int) (*bytes.Buffer, error) {
	img := image.NewRGBA(image.Rect(0, 0, outW, outH))
	// 背景
	draw.Draw(img, img.Bounds(), &image.Uniform{C: color.RGBA{245, 245, 245, 255}}, image.Point{}, draw.Src)

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

	// 各セルの矩形サイズ
	cellW := outW / gridW
	cellH := outH / gridH
	if cellW < 1 {
		cellW = 1
	}
	if cellH < 1 {
		cellH = 1
	}

	// 着色
	for gy := 0; gy < gridH; gy++ {
		for gx := 0; gx < gridW; gx++ {
			v := counts[gy*gridW+gx]
			col := heatColor(float64(v) / float64(maxv))
			rect := image.Rect(gx*cellW, gy*cellH, (gx+1)*cellW, (gy+1)*cellH)
			draw.Draw(img, rect, &image.Uniform{C: col}, image.Point{}, draw.Src)
		}
	}

	buf := &bytes.Buffer{}
	if err := png.Encode(buf, img); err != nil {
		return nil, err
	}
	return buf, nil
}

// heatColor 正規化値(0..1)から擬似カラー(青→赤→黄)
func heatColor(t float64) color.RGBA {
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	// 青(0,0,255)→赤(255,0,0)→黄(255,255,0)
	if t < 0.5 {
		u := t / 0.5
		r := uint8(255 * u)
		g := uint8(0)
		b := uint8(255 * (1 - u))
		return color.RGBA{r, g, b, 255}
	}
	u := (t - 0.5) / 0.5
	r := uint8(255)
	g := uint8(255 * u)
	b := uint8(0)
	return color.RGBA{r, g, b, 255}
}
