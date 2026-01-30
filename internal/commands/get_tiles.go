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
	"sync"
	"time"
)

const (
	getTileCacheTTL      = 2 * time.Minute
	getTileMaxConcurrent = 16
)

var tileHTTPClient = &http.Client{
	Timeout: 12 * time.Second,
	Transport: &http.Transport{
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   32,
		MaxConnsPerHost:       32,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	},
}

type tileCacheEntry struct {
	data      []byte
	expiresAt time.Time
}

var tileCache struct {
	mu    sync.Mutex
	items map[string]tileCacheEntry
}

func init() {
	tileCache.items = make(map[string]tileCacheEntry)
}

// downloadTile 単一のタイル画像をダウンロードするヘルパー関数
func (c *GetCommand) downloadTile(ctx context.Context, tlx, tly int) ([]byte, error) {
	url := fmt.Sprintf("https://backend.wplace.live/files/s0/tiles/%d/%d.png", tlx, tly)
	cacheKey := fmt.Sprintf("%d-%d", tlx, tly)
	if data, ok := getTileFromCache(cacheKey); ok {
		return data, nil
	}

	val, err := c.limiter.Do(ctx, "backend.wplace.live", func() (interface{}, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		resp, err := tileHTTPClient.Do(req)
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
	data, ok := val.([]byte)
	if !ok {
		return nil, fmt.Errorf("unexpected response type for tile %d-%d", tlx, tly)
	}
	if len(data) > 0 {
		storeTileCache(cacheKey, data)
	}
	return data, nil
}

func getTileFromCache(key string) ([]byte, bool) {
	tileCache.mu.Lock()
	defer tileCache.mu.Unlock()
	entry, ok := tileCache.items[key]
	if !ok {
		return nil, false
	}
	if time.Now().After(entry.expiresAt) {
		delete(tileCache.items, key)
		return nil, false
	}
	return entry.data, true
}

func storeTileCache(key string, data []byte) {
	tileCache.mu.Lock()
	defer tileCache.mu.Unlock()
	tileCache.items[key] = tileCacheEntry{
		data:      data,
		expiresAt: time.Now().Add(getTileCacheTTL),
	}
}

func (c *GetCommand) downloadTilesGrid(ctx context.Context, minX, minY, cols, rows int) ([][]byte, error) {
	total := cols * rows
	tiles := make([][]byte, total)
	sem := make(chan struct{}, getTileMaxConcurrent)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	for y := 0; y < rows; y++ {
		for x := 0; x < cols; x++ {
			tileX := minX + x
			tileY := minY + y
			idx := y*cols + x
			wg.Add(1)
			sem <- struct{}{}
			go func(ix, iy, index int) {
				defer wg.Done()
				defer func() { <-sem }()
				if ctx.Err() != nil {
					return
				}
				reqCtx, cancelReq := context.WithTimeout(ctx, 15*time.Second)
				data, err := c.downloadTile(reqCtx, ix, iy)
				cancelReq()
				if err != nil {
					mu.Lock()
					if firstErr == nil {
						firstErr = err
						cancel()
					}
					mu.Unlock()
					return
				}
				tiles[index] = data
			}(tileX, tileY, idx)
		}
	}
	wg.Wait()
	if firstErr != nil {
		return nil, firstErr
	}
	for i, data := range tiles {
		if len(data) == 0 {
			return nil, fmt.Errorf("タイル画像のダウンロードに失敗しました (index=%d)", i)
		}
	}
	return tiles, nil
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
