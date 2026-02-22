package utils

import "fmt"

const (
	MainMonitorTileX  = 1818
	MainMonitorTileY  = 806
	MainMonitorPixelX = 989
	MainMonitorPixelY = 358
	MainMonitorWidth  = 107
	MainMonitorHeight = 142
)

func BuildMainMonitorWplaceURL() string {
	centerAbsX := float64(MainMonitorTileX*WplaceTileSize+MainMonitorPixelX) + float64(MainMonitorWidth)/2
	centerAbsY := float64(MainMonitorTileY*WplaceTileSize+MainMonitorPixelY) + float64(MainMonitorHeight)/2
	centerTileX := int(centerAbsX) / WplaceTileSize
	centerTileY := int(centerAbsY) / WplaceTileSize
	centerPixelX := int(centerAbsX) % WplaceTileSize
	centerPixelY := int(centerAbsY) % WplaceTileSize
	center := TilePixelCenterToLngLat(centerTileX, centerTileY, centerPixelX, centerPixelY)
	return BuildWplaceURL(center.Lng, center.Lat, ZoomFromImageSize(MainMonitorWidth, MainMonitorHeight))
}

func MainMonitorFullsizeString() string {
	return fmt.Sprintf(
		"%d-%d-%d-%d-%d-%d",
		MainMonitorTileX,
		MainMonitorTileY,
		MainMonitorPixelX,
		MainMonitorPixelY,
		MainMonitorWidth,
		MainMonitorHeight,
	)
}
