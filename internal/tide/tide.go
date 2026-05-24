package tide

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

var httpClient = &http.Client{Timeout: 10 * time.Second}

// Extreme represents one tidal extreme (high or low water).
type Extreme struct {
	Time   time.Time
	Height float64
	Type   string // "high" or "low"
}

type sgTideResp struct {
	Data []struct {
		Height float64 `json:"height"`
		Time   string  `json:"time"`
		Type   string  `json:"type"` // "high" or "low"
	} `json:"data"`
	Errors map[string]any `json:"errors"`
	Meta   struct {
		Station struct {
			Name string `json:"name"`
		} `json:"station"`
	} `json:"meta"`
}

// Fetch retrieves today's tidal extremes from the Stormglass API.
func Fetch(lat, lon float64, apiKey string) ([]Extreme, error) {
	loc, _ := time.LoadLocation("Europe/London")
	now := time.Now().In(loc)
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc).UTC()
	end := start.Add(24 * time.Hour)

	url := fmt.Sprintf(
		"https://api.stormglass.io/v2/tide/extremes/point?lat=%.4f&lng=%.4f&start=%d&end=%d",
		lat, lon, start.Unix(), end.Unix(),
	)

	req, _ := http.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("Authorization", apiKey)
	req.Header.Set("User-Agent", "MeshCoreBots/1.0")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("stormglass tide request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var sg sgTideResp
	if err := json.Unmarshal(body, &sg); err != nil {
		return nil, fmt.Errorf("parsing stormglass tide response: %w", err)
	}
	if len(sg.Errors) > 0 {
		return nil, fmt.Errorf("stormglass tide API error: %v", sg.Errors)
	}

	extremes := make([]Extreme, 0, len(sg.Data))
	for _, e := range sg.Data {
		t, err := time.Parse("2006-01-02 15:04:05+00:00", e.Time)
		if err != nil {
			// Try RFC3339 fallback
			t, err = time.Parse(time.RFC3339, e.Time)
			if err != nil {
				continue
			}
		}
		extremes = append(extremes, Extreme{
			Time:   t.In(loc),
			Height: e.Height,
			Type:   e.Type,
		})
	}
	return extremes, nil
}

// Format returns a compact single-line tide summary (up to 3 extremes).
func Format(stationName string, extremes []Extreme) string {
	if len(extremes) == 0 {
		return fmt.Sprintf("🌊 %s: no tide data", stationName)
	}

	limit := 3
	if len(extremes) < limit {
		limit = len(extremes)
	}

	result := fmt.Sprintf("🌊 %s:", stationName)
	for i, e := range extremes[:limit] {
		label := "HW"
		if e.Type == "low" {
			label = "LW"
		}
		part := fmt.Sprintf("%s %02d:%02d %.1fm", label, e.Time.Hour(), e.Time.Minute(), e.Height)
		if i == 0 {
			result += " " + part
		} else {
			result += " · " + part
		}
	}
	return result
}
