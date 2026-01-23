package embeds

import (
	"Koukyo_discord_bot/internal/monitor"
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
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
	const margin = 40

	img := image.NewRGBA(image.Rect(0, 0, width, height))

	// 背景
	draw.Draw(img, img.Bounds(), &image.Uniform{C: color.RGBA{240, 240, 240, 255}}, image.Point{}, draw.Src)

	// 座標系領域
	plotRect := image.Rect(margin, margin, width-margin, height-margin)

	// 軸描画
	axisColor := color.RGBA{80, 80, 80, 255}
	// X軸
	for x := plotRect.Min.X; x <= plotRect.Max.X; x++ {
		img.Set(x, plotRect.Max.Y, axisColor)
	}
	// Y軸
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
	for _, r := range history {
		if r.Percentage > pMax {
			pMax = r.Percentage
		}
	}
	// 余白と目盛り最大
	pMax = math.Max(pMax, 1.0)

	// 軸目盛り（Y: 0, 中間, 最大）
	tickColor := color.RGBA{120, 120, 120, 255}
	yTicks := []float64{0, pMax / 2, pMax}
	for _, tv := range yTicks {
		y := plotRect.Max.Y - int(float64(plotRect.Dy())*(tv/pMax))
		// 短い横線
		for x := plotRect.Min.X - 6; x <= plotRect.Min.X; x++ {
			img.Set(x, y, tickColor)
		}
		// ラベル
		drawText5x7(img, plotRect.Min.X-35, y-4, fmt.Sprintf("%.0f%%", tv), color.RGBA{60, 60, 60, 255})
	}

	// X軸ラベル（開始/終了時刻）
	tMinStr := history[0].Timestamp.Format("15:04")
	tMaxStr := history[len(history)-1].Timestamp.Format("15:04")
	drawText5x7(img, plotRect.Min.X, plotRect.Max.Y+8, tMinStr, color.RGBA{60, 60, 60, 255})
	drawText5x7(img, plotRect.Max.X-28, plotRect.Max.Y+8, tMaxStr, color.RGBA{60, 60, 60, 255})

	// 線色
	lineColor := color.RGBA{99, 164, 255, 255}

	// 折れ線
	prevX, prevY := 0, 0
	for i, r := range history {
		// x: 時刻を線形
		x := plotRect.Min.X + int(float64(plotRect.Dx())*float64(r.Timestamp.Sub(tMin))/float64(tMax.Sub(tMin)))
		// y: 0..pMax を逆向きで
		y := plotRect.Max.Y - int(float64(plotRect.Dy())*(r.Percentage/pMax))

		// 点
		img.Set(x, y, lineColor)
		img.Set(x, y-1, lineColor)
		img.Set(x, y+1, lineColor)
		img.Set(x-1, y, lineColor)
		img.Set(x+1, y, lineColor)

		if i > 0 {
			// 前の点から線を引く（簡易Bresenham風）
			dx := x - prevX
			dy := y - prevY
			steps := int(math.Max(math.Abs(float64(dx)), math.Abs(float64(dy))))
			if steps == 0 {
				steps = 1
			}
			for s := 0; s <= steps; s++ {
				xi := prevX + int(float64(dx)*float64(s)/float64(steps))
				yi := prevY + int(float64(dy)*float64(s)/float64(steps))
				img.Set(xi, yi, lineColor)
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
