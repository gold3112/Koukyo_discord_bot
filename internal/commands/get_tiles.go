package commands

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/draw"
	"image/png"

	"Koukyo_discord_bot/internal/utils"
	"Koukyo_discord_bot/internal/wplace"
)

const getTileMaxConcurrent = 16

// downloadTile 単一のタイル画像をダウンロードする。
func (c *GetCommand) downloadTile(ctx context.Context, tileX, tileY int) ([]byte, error) {
	return wplace.DownloadTile(ctx, c.limiter, tileX, tileY)
}

func (c *GetCommand) downloadTilesGrid(ctx context.Context, minX, minY, cols, rows int) ([][]byte, error) {
	return wplace.DownloadTilesGrid(ctx, c.limiter, minX, minY, cols, rows, getTileMaxConcurrent)
}

// combineTiles 複数のタイル画像を結合しPNGバイトにする。
func combineTiles(tilesData [][]byte, tileWidth, tileHeight, gridCols, gridRows int) (*bytes.Buffer, error) {
	img, err := combineTilesImage(tilesData, tileWidth, tileHeight, gridCols, gridRows)
	if err != nil {
		return nil, err
	}
	buf := new(bytes.Buffer)
	if err := png.Encode(buf, img); err != nil {
		return nil, fmt.Errorf("結合画像のPNGエンコードに失敗しました: %w", err)
	}
	return buf, nil
}

func combineTilesImage(tilesData [][]byte, tileWidth, tileHeight, gridCols, gridRows int) (*image.RGBA, error) {
	nrgba, err := wplace.CombineTilesImage(tilesData, tileWidth, tileHeight, gridCols, gridRows)
	if err != nil {
		return nil, err
	}
	return nrgbaToRGBA(nrgba), nil
}

// combineTilesCropped combines tiles but only renders a cropped region to reduce memory usage.
// cropRect is relative to the combined image's coordinate system.
func combineTilesCropped(tilesData [][]byte, tileWidth, tileHeight, gridCols, gridRows int, cropRect image.Rectangle) (*image.RGBA, error) {
	nrgba, err := wplace.CombineTilesCroppedImage(tilesData, tileWidth, tileHeight, gridCols, gridRows, cropRect)
	if err != nil {
		return nil, err
	}
	return nrgbaToRGBA(nrgba), nil
}

func nrgbaToRGBA(src *image.NRGBA) *image.RGBA {
	dst := image.NewRGBA(src.Bounds())
	draw.Draw(dst, dst.Bounds(), src, src.Bounds().Min, draw.Src)
	return dst
}

func calculateZoomFromWH(width, height int) float64 {
	return utils.ZoomFromImageSize(width, height)
}
