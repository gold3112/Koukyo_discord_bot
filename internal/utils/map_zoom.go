package utils

import "math"

const (
	// Web Mercator tile size used by common web map renderers.
	webMercatorTileSize = 256.0
	// Assumed viewport for shared links (desktop-first).
	defaultViewportWidth  = 1280.0
	defaultViewportHeight = 720.0
	// Wplace UI reduces effective visible map area from raw viewport.
	uiWidthFactor  = 0.82
	uiHeightFactor = 0.90
	// Calibrates Wplace zoom behavior against desktop observations.
	zoomBias = -0.43
	// Wplace visibility floor (tiles can disappear below this).
	minSafeZoom = 10.7
	maxSafeZoom = 22.0
)

// ZoomFromImageSize calculates a deterministic zoom that fits an area into a
// default viewport, based on map geometry (not an empirical regression).
func ZoomFromImageSize(width, height int) float64 {
	if width <= 0 || height <= 0 {
		return minSafeZoom
	}

	worldCanvasPx := float64(WplaceTilesPerEdge * WplaceTileSize)
	fracW := float64(width) / worldCanvasPx
	fracH := float64(height) / worldCanvasPx
	if fracW <= 0 || fracH <= 0 {
		return minSafeZoom
	}

	usableW := defaultViewportWidth * uiWidthFactor
	usableH := defaultViewportHeight * uiHeightFactor
	if usableW <= 0 || usableH <= 0 {
		return minSafeZoom
	}

	zoomW := math.Log2(usableW / (webMercatorTileSize * fracW))
	zoomH := math.Log2(usableH / (webMercatorTileSize * fracH))
	zoom := math.Min(zoomW, zoomH) + zoomBias

	if math.IsNaN(zoom) || math.IsInf(zoom, 0) {
		return minSafeZoom
	}
	if zoom < minSafeZoom {
		return minSafeZoom
	}
	if zoom > maxSafeZoom {
		return maxSafeZoom
	}
	return zoom
}
