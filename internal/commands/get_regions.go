package commands

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const regionDBURL = "https://gold3112.online/tools/region-viewer/region_database.json"

type Region struct {
	RegionID     int             `json:"region_id"`
	CityID       int             `json:"city_id"`
	CountryID    int             `json:"country_id"`
	RegionCoords [2]int          `json:"region_coords"`
	TileRange    RegionTileRange `json:"tile_range"`
	CenterLatLng [2]float64      `json:"center_latlng"`
	ExpectedCity string          `json:"expected_city"`
	Name         string          `json:"name"`
}

type RegionDB map[string]Region

type RegionTileRange struct {
	Min [2]int `json:"min"`
	Max [2]int `json:"max"`
}

func loadRegionDB(path string) (RegionDB, error) {
	var bytes []byte
	var err error
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		bytes, err = fetchRegionDB(path)
	} else {
		bytes, err = os.ReadFile(path)
	}
	if err != nil {
		return nil, err
	}
	var db RegionDB
	if err := json.Unmarshal(bytes, &db); err != nil {
		return nil, err
	}
	return db, nil
}

func fetchRegionDB(url string) ([]byte, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("region database fetch failed: %s", resp.Status)
	}
	return io.ReadAll(resp.Body)
}

func findRegionByName(db RegionDB, name string) (Region, bool) {
	if reg, ok := db[name]; ok {
		return reg, true
	}
	for _, reg := range db {
		if strings.EqualFold(reg.Name, name) {
			return reg, true
		}
	}
	return Region{}, false
}
