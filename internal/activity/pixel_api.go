package activity

import (
	"compress/gzip"
	"compress/zlib"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"Koukyo_discord_bot/internal/utils"
)

// PixelAPIResponse represents the Wplace pixel API response payload.
type PixelAPIResponse struct {
	PaintedBy *PaintedBy `json:"paintedBy"`
}

// NewPixelHTTPClient returns a HTTP client configured for the pixel API.
func NewPixelHTTPClient() *http.Client {
	dialer := &net.Dialer{
		Timeout:   5 * time.Second,
		KeepAlive: 5 * time.Second,
	}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialer.DialContext(ctx, "tcp4", addr)
		},
		ForceAttemptHTTP2:  false,
		DisableKeepAlives:  true,
		DisableCompression: true,
		TLSNextProto:       make(map[string]func(string, *tls.Conn) http.RoundTripper),
	}
	return &http.Client{Transport: transport, Timeout: 8 * time.Second}
}

// FetchPixelInfo fetches pixel info from Wplace backend.
// Returns parsed response, HTTP status (0 if request failed before response), and error.
func FetchPixelInfo(
	ctx context.Context,
	client *http.Client,
	limiter *utils.RateLimiter,
	tileX, tileY, pixelX, pixelY int,
) (*PixelAPIResponse, int, error) {
	if client == nil {
		client = NewPixelHTTPClient()
	}
	url := fmt.Sprintf("https://backend.wplace.live/s0/pixel/%d/%d?x=%d&y=%d", tileX, tileY, pixelX, pixelY)

	doReq := func() (interface{}, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		req.Close = true
		req.Header.Set("Connection", "close")
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
		req.Header.Set("Accept", "application/json, text/plain, */*")
		req.Header.Set("Accept-Language", "en-US,en;q=0.9")
		req.Header.Set("Accept-Encoding", "identity")
		req.Header.Set("Sec-Fetch-Dest", "empty")
		req.Header.Set("Sec-Fetch-Mode", "cors")
		req.Header.Set("Sec-Fetch-Site", "same-site")
		req.Header.Set("Sec-CH-UA", "\"Chromium\";v=\"120\", \"Not=A?Brand\";v=\"24\", \"Google Chrome\";v=\"120\"")
		req.Header.Set("Sec-CH-UA-Mobile", "?0")
		req.Header.Set("Sec-CH-UA-Platform", "\"Windows\"")
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		body, err := readResponseBody(resp)
		if err != nil {
			return &pixelResponse{status: resp.StatusCode, body: nil}, err
		}
		return &pixelResponse{status: resp.StatusCode, body: body}, nil
	}

	var val interface{}
	var err error
	if limiter != nil {
		val, err = limiter.Do(ctx, "backend.wplace.live", doReq)
	} else {
		val, err = doReq()
	}
	if err != nil {
		return nil, 0, err
	}
	resp, ok := val.(*pixelResponse)
	if !ok || resp == nil {
		return nil, 0, fmt.Errorf("unexpected pixel response type")
	}
	if resp.status != http.StatusOK {
		return nil, resp.status, fmt.Errorf("pixel api status: %s body=%s", http.StatusText(resp.status), string(resp.body))
	}
	var parsed PixelAPIResponse
	if err := json.Unmarshal(resp.body, &parsed); err != nil {
		return nil, resp.status, err
	}
	return &parsed, resp.status, nil
}

type pixelResponse struct {
	status int
	body   []byte
}

func readResponseBody(resp *http.Response) ([]byte, error) {
	switch resp.Header.Get("Content-Encoding") {
	case "gzip":
		reader, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, err
		}
		defer reader.Close()
		return io.ReadAll(reader)
	case "deflate":
		reader, err := zlib.NewReader(resp.Body)
		if err != nil {
			return nil, err
		}
		defer reader.Close()
		return io.ReadAll(reader)
	default:
		return io.ReadAll(resp.Body)
	}
}
