package frost

import (
	"fmt"

	"meshcore-bots/internal/weather"
)

// CheckMorning returns true if today's minimum temperature is at or below the threshold.
func CheckMorning(d *weather.Data, thresholdC float64) bool {
	return d.MinTempC <= thresholdC
}

// CheckEvening returns true if tomorrow's minimum temperature is at or below the threshold.
func CheckEvening(d *weather.Data, thresholdC float64) bool {
	return d.TomorrowMinC <= thresholdC
}

// FormatMorning returns a morning frost warning message.
func FormatMorning(label string, d *weather.Data) string {
	return fmt.Sprintf("🧊 %s: Frost risk this morning (%.0f°C overnight). Check road conditions.",
		label, d.MinTempC)
}

// FormatEvening returns an evening frost forecast message.
func FormatEvening(label string, d *weather.Data) string {
	return fmt.Sprintf("🧊 %s: Frost forecast tonight (low %.0f°C). Expect icy roads tomorrow.",
		label, d.TomorrowMinC)
}
