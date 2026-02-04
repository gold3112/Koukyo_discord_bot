package utils

import "math"

const (
	// Web Mercator tile size used by common web map renderers.
	webMercatorTileSize = 256.0
	// Assumed viewport for shared links (desktop-first).
	defaultViewportWidth  = 1280.0
	defaultViewportHeight = 720.0
	// Keep some margin so the region is visible with breathing room.
	defaultViewportPadding = 0.90
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

	usableW := defaultViewportWidth * defaultViewportPadding
	usableH := defaultViewportHeight * defaultViewportPadding
	if usableW <= 0 || usableH <= 0 {
		return minSafeZoom
	}

	zoomW := math.Log2(usableW / (webMercatorTileSize * fracW))
	zoomH := math.Log2(usableH / (webMercatorTileSize * fracH))
	zoom := math.Min(zoomW, zoomH)

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
