package traffic

import (
	"crypto/sha256"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode"

	"meshcore-bots/internal/state"
)

const (
	incidentsURL = "https://www.traffic.gov.scot/traffic-information/incidents"
	maxIncidents = 3   // max incidents named in a summary
	maxMsgIncLen = 100 // max chars per incident before truncation
)

var (
	httpClient = &http.Client{Timeout: 15 * time.Second}
	h2FullRE = regexp.MustCompile(`(?i)<h2[^>]*>(.*?)</h2>`)
	tagRE    = regexp.MustCompile(`<[^>]+>`)
	spaceRE  = regexp.MustCompile(`\s+`)
)

// Incident is a parsed traffic incident.
type Incident struct {
	ID    string // stable dedup key
	Title string // h2 text: road + location
	Body  string // plain text of description block (used for emoji classification)
	Emoji string // incident-type emoji derived from Body
}

// FetchAll retrieves all current incidents from Traffic Scotland.
func FetchAll() ([]Incident, error) {
	req, err := http.NewRequest(http.MethodGet, incidentsURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "MeshCoreBots/1.0 (MeshCore mesh network bot)")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching traffic incidents: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return parseIncidents(string(body)), nil
}

// FilterByRoads returns incidents whose title matches any of the road names.
func FilterByRoads(incidents []Incident, roads []string) []Incident {
	var matched []Incident
	for _, inc := range incidents {
		if matchesAnyRoad(inc.Title, roads) {
			matched = append(matched, inc)
		}
	}
	return matched
}

// lowPriorityEmojis are incident types suppressed from summaries and alerts.
var lowPriorityEmojis = map[string]bool{
	"🔶": true, // lane restriction
	"↪": true, // diversion / contraflow
}

// FilterHighPriority removes low-priority incidents (lane restrictions, diversions).
// These are tracked for dedup but never sent to the mesh.
func FilterHighPriority(incidents []Incident) []Incident {
	var out []Incident
	for _, inc := range incidents {
		if !lowPriorityEmojis[inc.Emoji] {
			out = append(out, inc)
		}
	}
	return out
}

// DailySummary builds the morning summary, marks all incidents as seen, returns the message.
func DailySummary(label string, incidents []Incident, st *state.IncidentState) string {
	for _, inc := range incidents {
		st.MarkSeen(inc.ID)
	}
	return formatSummary(label, incidents)
}

// NewIncidents returns incidents not yet seen and marks them.
func NewIncidents(incidents []Incident, st *state.IncidentState) []Incident {
	var novel []Incident
	for _, inc := range incidents {
		if st.IsNew(inc.ID) {
			novel = append(novel, inc)
			st.MarkSeen(inc.ID)
		}
	}
	return novel
}

// FormatAlert returns a real-time new-incident alert using the incident's type emoji.
func FormatAlert(label string, inc Incident) string {
	return fmt.Sprintf("%s %s: New — %s", inc.Emoji, label, truncate(inc.Title, maxMsgIncLen))
}

func formatSummary(label string, incidents []Incident) string {
	if len(incidents) == 0 {
		return fmt.Sprintf("🟢 %s: No incidents on monitored roads", label)
	}

	parts := make([]string, 0, maxIncidents)
	for i, inc := range incidents {
		if i >= maxIncidents {
			break
		}
		parts = append(parts, inc.Emoji+" "+truncate(inc.Title, maxMsgIncLen))
	}

	extra := len(incidents) - maxIncidents
	msg := fmt.Sprintf("🚦 %s: %s", label, strings.Join(parts, ". "))
	if extra > 0 {
		msg += fmt.Sprintf(". +%d more", extra)
	}
	return msg
}

// parseIncidents extracts incidents from the Traffic Scotland HTML page.
// For each h2 heading (road + location) it also captures the body text
// between that h2 and the next one, which is used to derive the incident emoji.
func parseIncidents(htmlContent string) []Incident {
	// Find all h2 positions (start and end byte offsets in the HTML string).
	allIdx := h2FullRE.FindAllStringIndex(htmlContent, -1)
	allSub := h2FullRE.FindAllStringSubmatch(htmlContent, -1)

	var incidents []Incident
	seen := make(map[string]bool)

	for i, m := range allSub {
		raw := tagRE.ReplaceAllString(m[1], " ")
		title := htmlUnescape(strings.TrimSpace(spaceRE.ReplaceAllString(raw, " ")))

		if title == "" || !looksLikeRoadIncident(title) {
			continue
		}

		id := incidentID(title)
		if seen[id] {
			continue
		}
		seen[id] = true

		// Extract body text: from end of this h2 to start of the next h2 (or EOF).
		bodyStart := allIdx[i][1]
		bodyEnd := len(htmlContent)
		if i+1 < len(allIdx) {
			bodyEnd = allIdx[i+1][0]
		}
		rawBody := htmlContent[bodyStart:bodyEnd]
		body := htmlUnescape(strings.TrimSpace(spaceRE.ReplaceAllString(
			tagRE.ReplaceAllString(rawBody, " "), " ")))
		// Keep body concise for classification (first 300 chars is plenty).
		if len(body) > 300 {
			body = body[:300]
		}

		emoji := incidentEmoji(title, body)
		incidents = append(incidents, Incident{ID: id, Title: title, Body: body, Emoji: emoji})
	}
	return incidents
}

// incidentEmoji classifies an incident by scanning its title and description body
// for keywords, returning the most specific matching emoji.
func incidentEmoji(title, body string) string {
	combined := strings.ToLower(title + " " + body)
	switch {
	case anyContains(combined, "accident", "collision", "crash", "rta"):
		return "💥"
	case anyContains(combined, "road closed", "closure", "road closure"):
		return "🚫"
	case anyContains(combined, "flood", "flooding", "water on road"):
		return "🌊"
	case anyContains(combined, "ice", "frost", "black ice", "icy", "snow", "blizzard", "winter"):
		return "🧊"
	case anyContains(combined, "roadwork", "road work", "maintenance", "resurfac", "surfacing", "utility", "carriageway work"):
		return "🚧"
	case anyContains(combined, "breakdown", "broken down", "stricken vehicle", "vehicle fire"):
		return "🔧"
	case anyContains(combined, "lane closed", "lane restriction", "lane reduced", "lanes restrict"):
		return "🔶"
	case anyContains(combined, "congestion", "queuing", "queue", "heavy traffic", "slow traffic", "delays"):
		return "🐢"
	case anyContains(combined, "diversion", "contraflow"):
		return "↪"
	case anyContains(combined, "bridge", "signal", "traffic light", "traffic signal"):
		return "🚦"
	default:
		return "⚠"
	}
}

func anyContains(s string, keywords ...string) bool {
	for _, k := range keywords {
		if strings.Contains(s, k) {
			return true
		}
	}
	return false
}

func matchesAnyRoad(title string, roads []string) bool {
	upper := strings.ToUpper(title)
	for _, road := range roads {
		if roadWordMatch(upper, strings.ToUpper(road)) {
			return true
		}
	}
	return false
}

func roadWordMatch(title, road string) bool {
	start := 0
	for {
		idx := strings.Index(title[start:], road)
		if idx == -1 {
			return false
		}
		abs := start + idx
		if abs > 0 && isAlphanumeric(rune(title[abs-1])) {
			start = abs + 1
			continue
		}
		end := abs + len(road)
		if end < len(title) && isAlphanumeric(rune(title[end])) {
			start = abs + 1
			continue
		}
		return true
	}
}

func isAlphanumeric(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}

func looksLikeRoadIncident(title string) bool {
	prefixes := []string{"M", "A", "B", "Forth", "Tay", "Kincardine", "Queensferry", "Erskine", "Skye", "Dornoch", "Kessock"}
	upper := strings.ToUpper(title)
	for _, p := range prefixes {
		if strings.HasPrefix(upper, strings.ToUpper(p)) {
			return true
		}
	}
	return false
}

func incidentID(title string) string {
	h := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(title))))
	return fmt.Sprintf("%x", h[:8])
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	cut := s[:max]
	if idx := strings.LastIndex(cut, " "); idx > max/2 {
		cut = cut[:idx]
	}
	return cut + "…"
}

func htmlUnescape(s string) string {
	return strings.NewReplacer(
		"&amp;", "&", "&lt;", "<", "&gt;", ">",
		"&quot;", `"`, "&#39;", "'", "&#x27;", "'", "&nbsp;", " ",
	).Replace(s)
}

// LogIncidents is a debug helper.
func LogIncidents(label string, incidents []Incident) {
	log.Printf("Traffic [%s]: %d incidents", label, len(incidents))
}
