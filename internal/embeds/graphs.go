package embeds

import (
	"Koukyo_discord_bot/internal/monitor"
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"time"
)

// BuildDiffGraphPNG 差分履歴から簡易折れ線グラフPNGを生成
// history: 時系列の差分率, titleは埋め込み側で使う

func BuildDiffGraphPNG(history []monitor.DiffRecord) (*bytes.Buffer, error) {
	// 画像サイズ
	const width = 800
	const height = 400
	const margin = 56

	img := image.NewRGBA(image.Rect(0, 0, width, height))

	// 背景グラデーション
	for y := 0; y < height; y++ {
		c := uint8(240 - y*20/height)
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{c, c, 255, 255})
		}
	}

	// 座標系領域
	plotRect := image.Rect(margin, margin, width-margin, height-margin)

	// グリッド線
	gridColor := color.RGBA{210, 210, 240, 255}
	nYTicks := 5
	nXTicks := 6
	for i := 0; i <= nYTicks; i++ {
		y := plotRect.Max.Y - int(float64(plotRect.Dy())*float64(i)/float64(nYTicks))
		for x := plotRect.Min.X; x <= plotRect.Max.X; x++ {
			img.Set(x, y, gridColor)
		}
	}
	for i := 0; i <= nXTicks; i++ {
		x := plotRect.Min.X + int(float64(plotRect.Dx())*float64(i)/float64(nXTicks))
		for y := plotRect.Min.Y; y <= plotRect.Max.Y; y++ {
			img.Set(x, y, gridColor)
		}
	}

	// 軸描画
	axisColor := color.RGBA{60, 60, 60, 255}
	for x := plotRect.Min.X; x <= plotRect.Max.X; x++ {
		img.Set(x, plotRect.Max.Y, axisColor)
	}
	for y := plotRect.Min.Y; y <= plotRect.Max.Y; y++ {
		img.Set(plotRect.Min.X, y, axisColor)
	}

	if len(history) < 2 {
		// データが少ない場合は空のPNG返却
		buf := &bytes.Buffer{}
		if err := png.Encode(buf, img); err != nil {
			return nil, err
		}
		return buf, nil
	}

	// 範囲計算
	tMin := history[0].Timestamp
	tMax := history[len(history)-1].Timestamp
	if tMax.Before(tMin) {
		tMax = tMin.Add(time.Second)
	}
	pMax := 0.0
	pMin := 0.0
	for _, r := range history {
		if r.Percentage > pMax {
			pMax = r.Percentage
		}
		if r.Percentage < pMin {
			pMin = r.Percentage
		}
	}
	pMax = math.Max(pMax, 1.0)

	// Y軸目盛り
	tickColor := color.RGBA{100, 100, 120, 255}
	for i := 0; i <= nYTicks; i++ {
		v := pMin + (pMax-pMin)*float64(i)/float64(nYTicks)
		y := plotRect.Max.Y - int(float64(plotRect.Dy())*float64(i)/float64(nYTicks))
		// 目盛りラベル
		drawText5x7(img, plotRect.Min.X-44, y-4, fmt.Sprintf("%.1f%%", v), tickColor)
	}

	// X軸目盛り
	for i := 0; i <= nXTicks; i++ {
		t := tMin.Add(time.Duration(float64(tMax.Sub(tMin)) * float64(i) / float64(nXTicks)))
		x := plotRect.Min.X + int(float64(plotRect.Dx())*float64(i)/float64(nXTicks))
		drawText5x7(img, x-16, plotRect.Max.Y+10, t.Format("15:04"), tickColor)
	}

	// 軸ラベル
	drawText5x7(img, plotRect.Min.X+(plotRect.Dx()/2)-40, plotRect.Max.Y+32, "時刻", color.RGBA{40, 40, 80, 255})
	// 縦軸ラベル（縦書き風）
	yLabel := "差分率(%)"
	for i := 0; i < len(yLabel); i++ {
		drawText5x7(img, plotRect.Min.X-60, plotRect.Min.Y+20+i*10, string(yLabel[i]), color.RGBA{40, 40, 80, 255})
	}

	// 線色・太さ
	lineColor := color.RGBA{40, 120, 255, 255}
	pointColor := color.RGBA{0, 0, 0, 255}
	prevX, prevY := 0, 0
	for i, r := range history {
		x := plotRect.Min.X + int(float64(plotRect.Dx())*float64(r.Timestamp.Sub(tMin))/float64(tMax.Sub(tMin)))
		y := plotRect.Max.Y - int(float64(plotRect.Dy())*((r.Percentage-pMin)/(pMax-pMin)))

		// 太めの点
		for dx := -1; dx <= 1; dx++ {
			for dy := -1; dy <= 1; dy++ {
				img.Set(x+dx, y+dy, pointColor)
			}
		}

		if i > 0 {
			// 前の点から線を引く（太線）
			dx := x - prevX
			dy := y - prevY
			steps := int(math.Max(math.Abs(float64(dx)), math.Abs(float64(dy))))
			if steps == 0 {
				steps = 1
			}
			for s := 0; s <= steps; s++ {
				xi := prevX + int(float64(dx)*float64(s)/float64(steps))
				yi := prevY + int(float64(dy)*float64(s)/float64(steps))
				for wx := -1; wx <= 1; wx++ {
					for wy := -1; wy <= 1; wy++ {
						img.Set(xi+wx, yi+wy, lineColor)
					}
				}
			}
		}
		prevX, prevY = x, y
	}

	// エンコード
	buf := &bytes.Buffer{}
	if err := png.Encode(buf, img); err != nil {
		return nil, err
	}
	return buf, nil
}

// 5x7の簡易フォントで数字や記号を描く（0-9, :, ., %, h, m を想定）
func drawText5x7(img *image.RGBA, x, y int, s string, c color.RGBA) {
	for i := 0; i < len(s); i++ {
		ch := s[i]
		glyph := glyph5x7(ch)
		// 描画
		for gy := 0; gy < 7; gy++ {
			row := glyph[gy]
			for gx := 0; gx < 5; gx++ {
				if (row>>uint(4-gx))&1 == 1 {
					img.Set(x+gx+i*6, y+gy, c)
				}
			}
		}
	}
}

func glyph5x7(ch byte) [7]byte {
	// 各行5bit有効（MSBが左）
	switch ch {
	case '0':
		return [7]byte{0x1E, 0x11, 0x11, 0x11, 0x11, 0x11, 0x1E}
	case '1':
		return [7]byte{0x04, 0x06, 0x04, 0x04, 0x04, 0x04, 0x0E}
	case '2':
		return [7]byte{0x1E, 0x01, 0x01, 0x1E, 0x10, 0x10, 0x1F}
	case '3':
		return [7]byte{0x1E, 0x01, 0x01, 0x0E, 0x01, 0x01, 0x1E}
	case '4':
		return [7]byte{0x10, 0x10, 0x11, 0x11, 0x1F, 0x01, 0x01}
	case '5':
		return [7]byte{0x1F, 0x10, 0x10, 0x1E, 0x01, 0x01, 0x1E}
	case '6':
		return [7]byte{0x0E, 0x10, 0x10, 0x1E, 0x11, 0x11, 0x1E}
	case '7':
		return [7]byte{0x1F, 0x01, 0x01, 0x02, 0x04, 0x08, 0x08}
	case '8':
		return [7]byte{0x1E, 0x11, 0x11, 0x1E, 0x11, 0x11, 0x1E}
	case '9':
		return [7]byte{0x1E, 0x11, 0x11, 0x1F, 0x01, 0x01, 0x0E}
	case ':':
		return [7]byte{0x00, 0x04, 0x00, 0x00, 0x04, 0x00, 0x00}
	case '.':
		return [7]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x06, 0x06}
	case '%':
		return [7]byte{0x18, 0x19, 0x02, 0x04, 0x08, 0x13, 0x03}
	case 'h':
		return [7]byte{0x10, 0x10, 0x10, 0x1E, 0x11, 0x11, 0x11}
	case 'm':
		return [7]byte{0x00, 0x00, 0x1E, 0x15, 0x15, 0x15, 0x15}
	default:
		return [7]byte{0, 0, 0, 0, 0, 0, 0}
	}
}
