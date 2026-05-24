package tide

import (
	"encoding/json"
	"log"
	"os"
	"sync"
	"time"
)

type tideEntry struct {
	Date      string    `json:"date"`
	FetchedAt time.Time `json:"fetched_at"`
	Extremes  []Extreme `json:"extremes"`
}

// Cache is a file-backed daily tide cache.
// Tidal extremes are astronomical and don't change, so one fetch per day per
// region is sufficient. Entries from previous days are silently ignored.
type Cache struct {
	mu      sync.Mutex
	path    string
	entries map[string]tideEntry
}

// NewCache creates (and pre-loads from disk if present) a daily tide cache.
func NewCache(path string) *Cache {
	c := &Cache{
		path:    path,
		entries: make(map[string]tideEntry),
	}
	c.load()
	return c
}

// FetchCached returns cached tidal extremes for cacheKey if stored for today,
// otherwise calls Fetch, caches the result, and returns it.
func FetchCached(cacheKey string, lat, lon float64, apiKey string, c *Cache) ([]Extreme, error) {
	if c == nil {
		return Fetch(lat, lon, apiKey)
	}

	today := todayDate()
	key := cacheKey + ":" + today

	c.mu.Lock()
	entry, ok := c.entries[key]
	c.mu.Unlock()

	if ok && entry.Date == today {
		log.Printf("Tide cache hit for %s (%s)", cacheKey, today)
		return entry.Extremes, nil
	}

	extremes, err := Fetch(lat, lon, apiKey)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.entries[key] = tideEntry{Date: today, FetchedAt: time.Now().UTC(), Extremes: extremes}
	c.mu.Unlock()

	c.save()
	return extremes, nil
}

func (c *Cache) load() {
	raw, err := os.ReadFile(c.path)
	if os.IsNotExist(err) {
		return
	}
	if err != nil {
		log.Printf("Tide cache: could not read %s: %v", c.path, err)
		return
	}

	var stored map[string]tideEntry
	if err := json.Unmarshal(raw, &stored); err != nil {
		log.Printf("Tide cache: could not parse %s: %v (starting fresh)", c.path, err)
		return
	}

	today := todayDate()
	kept := 0
	for k, v := range stored {
		if v.Date == today {
			c.entries[k] = v
			kept++
		}
	}
	if len(stored) > 0 {
		log.Printf("Tide cache: loaded %d/%d entries from %s (today=%s)",
			kept, len(stored), c.path, today)
	}
}

func (c *Cache) save() {
	c.mu.Lock()
	data, err := json.MarshalIndent(c.entries, "", "  ")
	c.mu.Unlock()

	if err != nil {
		log.Printf("Tide cache: marshal error: %v", err)
		return
	}
	if err := os.WriteFile(c.path, data, 0o644); err != nil {
		log.Printf("Tide cache: could not write %s: %v", c.path, err)
	}
}

func todayDate() string {
	loc, err := time.LoadLocation("Europe/London")
	if err != nil {
		loc = time.Local
	}
	return time.Now().In(loc).Format("2006-01-02")
}
