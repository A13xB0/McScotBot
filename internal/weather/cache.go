package weather

import (
	"encoding/json"
	"log"
	"os"
	"sync"
	"time"
)

type cacheEntry struct {
	Date      string    `json:"date"`
	FetchedAt time.Time `json:"fetched_at"`
	Data      *Data     `json:"data"`
}

// Cache is a file-backed daily weather cache keyed by "location:YYYY-MM-DD".
// Entries from previous days are silently ignored — no explicit eviction needed.
type Cache struct {
	mu      sync.Mutex
	path    string
	entries map[string]cacheEntry
}

// NewCache creates (and pre-loads from disk if present) a daily weather cache.
func NewCache(path string) *Cache {
	c := &Cache{
		path:    path,
		entries: make(map[string]cacheEntry),
	}
	c.load()
	return c
}

// FetchCached returns cached weather for location if stored for today,
// otherwise calls Fetch(location), caches the result, and returns it.
// location is both the wttr.in query string and the cache key.
func FetchCached(location string, c *Cache) (*Data, error) {
	if c == nil {
		return Fetch(location)
	}

	today := todayDate()
	key := location + ":" + today

	c.mu.Lock()
	entry, ok := c.entries[key]
	c.mu.Unlock()

	if ok && entry.Date == today {
		log.Printf("Weather cache hit for %s (%s)", location, today)
		return entry.Data, nil
	}

	data, err := Fetch(location)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.entries[key] = cacheEntry{Date: today, FetchedAt: time.Now().UTC(), Data: data}
	c.mu.Unlock()

	c.save()
	return data, nil
}

func (c *Cache) load() {
	raw, err := os.ReadFile(c.path)
	if os.IsNotExist(err) {
		return
	}
	if err != nil {
		log.Printf("Weather cache: could not read %s: %v", c.path, err)
		return
	}

	var stored map[string]cacheEntry
	if err := json.Unmarshal(raw, &stored); err != nil {
		log.Printf("Weather cache: could not parse %s: %v (starting fresh)", c.path, err)
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
		log.Printf("Weather cache: loaded %d/%d entries from %s (today=%s)",
			kept, len(stored), c.path, today)
	}
}

func (c *Cache) save() {
	c.mu.Lock()
	data, err := json.MarshalIndent(c.entries, "", "  ")
	c.mu.Unlock()

	if err != nil {
		log.Printf("Weather cache: marshal error: %v", err)
		return
	}
	if err := os.WriteFile(c.path, data, 0o644); err != nil {
		log.Printf("Weather cache: could not write %s: %v", c.path, err)
	}
}

func todayDate() string {
	loc, err := time.LoadLocation("Europe/London")
	if err != nil {
		loc = time.Local
	}
	return time.Now().In(loc).Format("2006-01-02")
}
