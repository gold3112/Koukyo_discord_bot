package notifications

import (
	"bytes"
	"fmt"
	"image/png"

	"Koukyo_discord_bot/internal/utils"
)

func smallDiffCoordinateLines(diffPNG []byte, limit int) ([]string, error) {
	coords, err := smallDiffCoordinatesFromDiffPNG(diffPNG, limit)
	if err != nil {
		return nil, err
	}
	if len(coords) == 0 {
		return nil, nil
	}

	lines := make([]string, 0, len(coords))
	for _, coord := range coords {
		url := utils.BuildWplaceHighDetailPixelURL(coord)
		lines = append(lines, fmt.Sprintf("- (%s:<%s>)", utils.FormatHyphenCoords(coord), url))
	}
	return lines, nil
}

func smallDiffCoordinatesFromDiffPNG(diffPNG []byte, limit int) ([]*utils.Coordinate, error) {
	if len(diffPNG) == 0 || limit <= 0 {
		return nil, nil
	}

	img, err := png.Decode(bytes.NewReader(diffPNG))
	if err != nil {
		return nil, err
	}

	b := img.Bounds()
	if b.Dx() <= 0 || b.Dy() <= 0 {
		return nil, nil
	}

	baseAbsX := utils.MainMonitorTileX*utils.WplaceTileSize + utils.MainMonitorPixelX
	baseAbsY := utils.MainMonitorTileY*utils.WplaceTileSize + utils.MainMonitorPixelY

	out := make([]*utils.Coordinate, 0, limit)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			_, _, _, a := img.At(x, y).RGBA()
			if a == 0 {
				continue
			}

			relX := x - b.Min.X
			relY := y - b.Min.Y
			out = append(out, absoluteToCoordinate(baseAbsX+relX, baseAbsY+relY))
			if len(out) >= limit {
				return out, nil
			}
		}
	}
	return out, nil
}

func absoluteToCoordinate(absX, absY int) *utils.Coordinate {
	return &utils.Coordinate{
		TileX:  absX / utils.WplaceTileSize,
		TileY:  absY / utils.WplaceTileSize,
		PixelX: absX % utils.WplaceTileSize,
		PixelY: absY % utils.WplaceTileSize,
	}
}
