package weather

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

var httpClient = &http.Client{Timeout: 10 * time.Second}

// Data holds the parsed weather for one location.
type Data struct {
	Condition    string
	MaxTempC     float64
	MinTempC     float64
	PrecipMmh    float64
	WindKmph     float64
	WindDir      string
	TomorrowMinC float64
}

type wttrResponse struct {
	Weather []struct {
		MaxtempC string `json:"maxtempC"`
		MintempC string `json:"mintempC"`
		Hourly   []struct {
			Time        string `json:"time"`
			WeatherDesc []struct {
				Value string `json:"value"`
			} `json:"weatherDesc"`
			ChanceOfRain  string `json:"chanceofrain"`
			WindspeedKmph string `json:"windspeedKmph"`
			WindDir16Pt   string `json:"winddir16Point"`
			PrecipMM      string `json:"precipMM"`
		} `json:"hourly"`
	} `json:"weather"`
}

// Fetch retrieves weather data for a named location from wttr.in.
func Fetch(location string) (*Data, error) {
	url := fmt.Sprintf("https://wttr.in/%s?format=j1", strings.ReplaceAll(location, " ", "+"))
	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetching weather for %s: %w", location, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var wt wttrResponse
	if err := json.Unmarshal(body, &wt); err != nil {
		return nil, fmt.Errorf("parsing wttr.in response: %w", err)
	}
	if len(wt.Weather) == 0 {
		return nil, fmt.Errorf("no weather data returned for %s", location)
	}

	today := wt.Weather[0]
	maxC, _ := strconv.ParseFloat(today.MaxtempC, 64)
	minC, _ := strconv.ParseFloat(today.MintempC, 64)

	// 09:00 slot (index 3: time="900") for morning conditions.
	slotIdx := 3
	if slotIdx >= len(today.Hourly) {
		slotIdx = 0
	}
	slot := today.Hourly[slotIdx]

	cond := ""
	if len(slot.WeatherDesc) > 0 {
		cond = strings.TrimSpace(slot.WeatherDesc[0].Value)
	}
	windKmph, _ := strconv.ParseFloat(slot.WindspeedKmph, 64)
	precipMM, _ := strconv.ParseFloat(slot.PrecipMM, 64)

	tomorrowMin := minC
	if len(wt.Weather) > 1 {
		tomorrowMin, _ = strconv.ParseFloat(wt.Weather[1].MintempC, 64)
	}

	return &Data{
		Condition:    cond,
		MaxTempC:     maxC,
		MinTempC:     minC,
		PrecipMmh:    precipMM,
		WindKmph:     windKmph,
		WindDir:      slot.WindDir16Pt,
		TomorrowMinC: tomorrowMin,
	}, nil
}

// Format returns a compact single-line weather summary.
func Format(label string, d *Data) string {
	cond := strings.TrimSpace(d.Condition)
	emoji := conditionEmoji(cond)
	windStr := fmt.Sprintf("%s %.0fkm/h", d.WindDir, d.WindKmph)
	if d.PrecipMmh >= 0.1 {
		return fmt.Sprintf("%s %s: %s, Hi %.0f°C Lo %.0f°C, %s, %.1fmm",
			emoji, label, cond, d.MaxTempC, d.MinTempC, windStr, d.PrecipMmh)
	}
	return fmt.Sprintf("%s %s: %s, Hi %.0f°C Lo %.0f°C, %s",
		emoji, label, cond, d.MaxTempC, d.MinTempC, windStr)
}

// conditionEmoji maps a condition string to a self-contained emoji
// (no variation selectors) so terminals always render them correctly.
func conditionEmoji(cond string) string {
	c := strings.ToLower(cond)
	switch {
	case strings.Contains(c, "thunder"):
		return "⛈"
	case strings.Contains(c, "heavy rain"), strings.Contains(c, "heavy snow"):
		return "🌧"
	case strings.Contains(c, "rain"), strings.Contains(c, "shower"), strings.Contains(c, "drizzle"):
		return "🌦"
	case strings.Contains(c, "snow"), strings.Contains(c, "sleet"), strings.Contains(c, "blizzard"):
		return "🌨"
	case strings.Contains(c, "fog"), strings.Contains(c, "mist"), strings.Contains(c, "haze"):
		return "🌫"
	case strings.Contains(c, "sunny"), strings.Contains(c, "clear"):
		return "☀"
	case strings.Contains(c, "partly cloudy"), strings.Contains(c, "partly sunny"):
		return "⛅"
	case strings.Contains(c, "mostly cloudy"):
		return "🌥"
	case strings.Contains(c, "overcast"), strings.Contains(c, "cloudy"):
		return "☁"
	default:
		return "🌡"
	}
}

var compassPoints = [16]string{
	"N", "NNE", "NE", "ENE", "E", "ESE", "SE", "SSE",
	"S", "SSW", "SW", "WSW", "W", "WNW", "NW", "NNW",
}

func degreesToCompass(deg float64) string {
	idx := int(math.Round(deg/22.5)) % 16
	if idx < 0 {
		idx += 16
	}
	return compassPoints[idx]
}

// keep degreesToCompass available for potential future use
var _ = degreesToCompass
