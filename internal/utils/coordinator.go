package utils

import (
	"fmt"
	"math"
)

const (
	// WplaceZoom Wplaceのズームレベル
	WplaceZoom = 11
	// WplaceTileSize タイル1枚のサイズ (px)
	WplaceTileSize = 1000
	// WplaceTilesPerEdge 1辺のタイル数 = 2^zoom
	WplaceTilesPerEdge = 1 << WplaceZoom // 2048
)

// Coordinate 座標データ
type Coordinate struct {
	TileX  int
	TileY  int
	PixelX int
	PixelY int
}

// LngLat 経度緯度
type LngLat struct {
	Lng float64
	Lat float64
}

// LngLatToTilePixel 経度緯度からタイル座標とピクセル座標を計算
func LngLatToTilePixel(lng, lat float64) *Coordinate {
	n := float64(WplaceTilesPerEdge)

	// 経度からX座標
	tileXFloat := (lng + 180) / 360 * n
	tileX := int(tileXFloat)
	pixelX := int((tileXFloat - float64(tileX)) * WplaceTileSize)

	// 緯度からY座標（Webメルカトル投影）
	latRad := lat * math.Pi / 180
	tileYFloat := (1 - math.Asinh(math.Tan(latRad))/math.Pi) / 2 * n
	tileY := int(tileYFloat)
	pixelY := int((tileYFloat - float64(tileY)) * WplaceTileSize)

	return &Coordinate{
		TileX:  tileX,
		TileY:  tileY,
		PixelX: pixelX,
		PixelY: pixelY,
	}
}

// TilePixelToLngLat タイル座標とピクセル座標から経度緯度を計算
func TilePixelToLngLat(tileX, tileY, pixelX, pixelY int) *LngLat {
	n := float64(WplaceTilesPerEdge)

	// タイルとピクセルを合わせた位置
	xFloat := float64(tileX) + float64(pixelX)/WplaceTileSize
	yFloat := float64(tileY) + float64(pixelY)/WplaceTileSize

	// 経度
	lng := xFloat/n*360 - 180

	// 緯度（Webメルカトル投影の逆変換）
	latRad := math.Atan(math.Sinh(math.Pi * (1 - 2*yFloat/n)))
	lat := latRad * 180 / math.Pi

	return &LngLat{
		Lng: lng,
		Lat: lat,
	}
}

// BuildWplaceURL Wplace.liveのURLを生成
func BuildWplaceURL(lng, lat, zoom float64) string {
	return fmt.Sprintf("https://wplace.live/?lat=%.6f&lng=%.6f&zoom=%.2f",
		lat, lng, zoom)
}

// FormatHyphenCoords ハイフン形式の座標文字列を生成
func FormatHyphenCoords(coord *Coordinate) string {
	return fmt.Sprintf("%d-%d-%d-%d",
		coord.TileX, coord.TileY, coord.PixelX, coord.PixelY)
}
