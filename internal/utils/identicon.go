package utils

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
)

const DefaultIdenticonSize = 160

// BuildIdenticonPNG generates a deterministic identicon for the given seed.
func BuildIdenticonPNG(seed string, size int) ([]byte, error) {
	if size <= 0 {
		size = DefaultIdenticonSize
	}
	hash := simpleHash(seed)
	hue := float64(hash%identiconColorsNb) * (360.0 / float64(identiconColorsNb))
	r, g, b := hslToRGB(hue, float64(identiconSaturation), float64(identiconLightness))
	fill := color.RGBA{R: r, G: g, B: b, A: 0xFF}

	img := image.NewRGBA(image.Rect(0, 0, size, size))
	cell := size / 5
	if cell <= 0 {
		cell = 1
	}
	for i := 0; i < 25; i++ {
		if (hash & (1 << (i % 15))) == 0 {
			continue
		}
		x := i / 5
		if i > 14 {
			x = 7 - x
		}
		y := i % 5
		startX := x * cell
		startY := y * cell
		for py := startY; py < startY+cell && py < size; py++ {
			for px := startX; px < startX+cell && px < size; px++ {
				img.Set(px, py, fill)
			}
		}
	}
	buf := new(bytes.Buffer)
	if err := png.Encode(buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

const (
	identiconColorsNb    = 9
	identiconSaturation  = 95
	identiconLightness   = 45
	identiconMagicNumber = 5
)

func simpleHash(value string) uint32 {
	hash := int32(identiconMagicNumber)
	for _, r := range value {
		hash = (hash ^ int32(r)) * -identiconMagicNumber
	}
	return uint32(hash) >> 2
}

func hslToRGB(h, s, l float64) (uint8, uint8, uint8) {
	h = math.Mod(h, 360) / 360.0
	s = clamp01(s / 100.0)
	l = clamp01(l / 100.0)
	if s == 0 {
		v := uint8(math.Round(l * 255))
		return v, v, v
	}
	var q float64
	if l < 0.5 {
		q = l * (1 + s)
	} else {
		q = l + s - l*s
	}
	p := 2*l - q
	r := hueToRGB(p, q, h+1.0/3.0)
	g := hueToRGB(p, q, h)
	b := hueToRGB(p, q, h-1.0/3.0)
	return uint8(math.Round(r * 255)), uint8(math.Round(g * 255)), uint8(math.Round(b * 255))
}

func hueToRGB(p, q, t float64) float64 {
	if t < 0 {
		t += 1
	}
	if t > 1 {
		t -= 1
	}
	switch {
	case t < 1.0/6.0:
		return p + (q-p)*6*t
	case t < 1.0/2.0:
		return q
	case t < 2.0/3.0:
		return p + (q-p)*(2.0/3.0-t)*6
	default:
		return p
	}
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
