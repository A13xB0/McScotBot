package config

import (
	"fmt"
	"log"
	"os"

	"gopkg.in/yaml.v3"
)

type Region struct {
	Scope           string   `yaml:"scope"`
	Label           string   `yaml:"label"`
	Lat             float64  `yaml:"lat"`             // used for sun calculation and tide API
	Lon             float64  `yaml:"lon"`
	WeatherLocation string   `yaml:"weather_location"` // wttr.in location name, e.g. "Glenrothes"
	TideStation     string   `yaml:"tide_station"`     // display name on tide messages
	TrafficRoads    []string `yaml:"traffic_roads"`
}

// WeatherQuery returns the location string to pass to wttr.in.
// Falls back to the region label if weather_location is not set.
func (r *Region) WeatherQuery() string {
	if r.WeatherLocation != "" {
		return r.WeatherLocation
	}
	return r.Label
}

type Config struct {
	RemotetermURL        string   `yaml:"remoteterm_url"`
	WeatherChannel       string   `yaml:"weather_channel"`
	TrafficChannel       string   `yaml:"traffic_channel"`
	DailyTime            string   `yaml:"daily_time"`
	FrostEveningTime     string   `yaml:"frost_evening_time"`
	PollIntervalMinutes  int      `yaml:"poll_interval_minutes"`
	MaxMessagesPerMinute int      `yaml:"max_messages_per_minute"`
	FrostThresholdC   float64  `yaml:"frost_threshold_c"`
	StormglassAPIKey  string   `yaml:"stormglass_api_key"`
	Regions           []Region `yaml:"regions"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config %s: %w", path, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	return cfg.withDefaults(), nil
}

func (c *Config) withDefaults() *Config {
	if c.DailyTime == "" {
		c.DailyTime = "07:00"
	}
	if c.FrostEveningTime == "" {
		c.FrostEveningTime = "17:00"
	}
	if c.PollIntervalMinutes <= 0 {
		c.PollIntervalMinutes = 15
	}
	if c.MaxMessagesPerMinute <= 0 {
		c.MaxMessagesPerMinute = 4
	}
	if c.MaxMessagesPerMinute > 10 {
		c.MaxMessagesPerMinute = 10
	}
	if c.FrostThresholdC == 0 {
		c.FrostThresholdC = 2.0
	}
	if c.WeatherChannel == "" {
		c.WeatherChannel = "#weather"
	}
	if c.TrafficChannel == "" {
		c.TrafficChannel = "#traffic"
	}
	for _, r := range c.Regions {
		if r.Lat == 0 && r.Lon == 0 {
			log.Printf("WARN region %q has lat=0 lon=0 — did you forget to set coordinates in config.yaml?", r.Label)
		}
	}
	return c
}
