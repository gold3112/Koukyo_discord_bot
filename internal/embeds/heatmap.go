package embeds

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"sort"
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

	// 最大値（外れ値を抑えるため95パーセンタイル）
	var nonZero []uint32
	for _, v := range counts {
		if v > 0 {
			nonZero = append(nonZero, v)
		}
	}
	if len(nonZero) == 0 {
		buf := &bytes.Buffer{}
		if err := png.Encode(buf, img); err != nil {
			return nil, err
		}
		return buf, nil
	}
	sort.Slice(nonZero, func(i, j int) bool { return nonZero[i] < nonZero[j] })
	idx := int(math.Floor(float64(len(nonZero)-1) * 0.95))
	if idx < 0 {
		idx = 0
	}
	maxv := nonZero[idx]
	if maxv == 0 {
		maxv = 1
	}

	// 着色
	for gy := 0; gy < gridH; gy++ {
		for gx := 0; gx < gridW; gx++ {
			v := counts[gy*gridW+gx]
			if v == 0 {
				continue
			}
			norm := math.Log1p(float64(v)) / math.Log1p(float64(maxv))
			if norm > 1 {
				norm = 1
			}
			norm = math.Pow(norm, 0.8)
			col := heatColor(norm)
			x0 := int(float64(gx) * float64(outW) / float64(gridW))
			y0 := int(float64(gy) * float64(outH) / float64(gridH))
			x1 := int(float64(gx+1) * float64(outW) / float64(gridW))
			y1 := int(float64(gy+1) * float64(outH) / float64(gridH))
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

// heatColor 正規化値(0..1)から擬似カラー(黒→赤→橙→黄)
func heatColor(t float64) color.RGBA {
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	// 黒(0,0,0)→赤(255,0,0)→橙(255,128,0)→黄(255,200,0)
	if t < 0.5 {
		u := t / 0.5
		r := uint8(255 * u)
		return color.RGBA{r, 0, 0, 255}
	}
	u := (t - 0.5) / 0.5
	r := uint8(255)
	g := uint8(128 + 72*u)
	return color.RGBA{r, g, 0, 255}
}
