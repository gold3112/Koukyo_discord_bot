package commands

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"io"
	"math"
	"net/http"
)

// downloadTile 単一のタイル画像をダウンロードするヘルパー関数
func (c *GetCommand) downloadTile(ctx context.Context, tlx, tly int) ([]byte, error) {
	url := fmt.Sprintf("https://backend.wplace.live/files/s0/tiles/%d/%d.png", tlx, tly)

	val, err := c.limiter.Do(ctx, "backend.wplace.live", func() (interface{}, error) {
		resp, err := http.Get(url)
		if err != nil {
			return nil, fmt.Errorf("HTTP GET failed for %s: %w", url, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("failed to download tile %d-%d (URL: %s), status: %s", tlx, tly, url, resp.Status)
		}
		return io.ReadAll(resp.Body)
	})
	if err != nil {
		return nil, err
	}
	return val.([]byte), nil
}

// combineTiles 複数のタイル画像を結合するヘルパー関数
// tilesData: 各タイル画像のバイトスライス
// tileWidth, tileHeight: 単一タイルの幅と高さ (ピクセル)
// gridCols, gridRows: タイルを配置するグリッドの列数と行数
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
	if len(tilesData) == 0 {
		return nil, fmt.Errorf("結合する画像データがありません")
	}
	if len(tilesData) != gridCols*gridRows {
		return nil, fmt.Errorf("画像データの数 (%d) がグリッドサイズ (%d x %d) と一致しません", len(tilesData), gridCols, gridRows)
	}

	combinedWidth := tileWidth * gridCols
	combinedHeight := tileHeight * gridRows
	combinedImg := image.NewRGBA(image.Rect(0, 0, combinedWidth, combinedHeight))

	for i, data := range tilesData {
		tileImg, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("タイル画像のデコードに失敗しました (インデックス %d): %w", i, err)
		}

		col := i % gridCols
		row := i / gridCols

		dp := image.Pt(col*tileWidth, row*tileHeight)
		draw.Draw(combinedImg, tileImg.Bounds().Add(dp), tileImg, image.Point{}, draw.Src)
	}

	return combinedImg, nil
}

func calculateZoomFromWH(width, height int) float64 {
	a := 21.16849365
	bw := -0.45385241
	bh := -2.76763227
	raw := a + bw*math.Log10(float64(width)) + bh*math.Log10(float64(height))
	if raw < 10.7 {
		return 10.7
	}
	return raw
}
