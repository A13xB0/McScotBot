package state

import "sync"

// IncidentState tracks which incident IDs have already been sent.
// The daily 07:00 run calls Reset() before the morning summary so the
// summary always goes out in full, and subsequent polls only alert on
// genuinely new incidents.
type IncidentState struct {
	mu   sync.Mutex
	seen map[string]bool
}

func New() *IncidentState {
	return &IncidentState{seen: make(map[string]bool)}
}

// Reset clears all seen incident IDs. Call at the start of each daily run.
func (s *IncidentState) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seen = make(map[string]bool)
}

// MarkSeen records that an incident ID has been announced.
func (s *IncidentState) MarkSeen(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seen[id] = true
}

// IsNew returns true if this incident ID has not been seen before.
func (s *IncidentState) IsNew(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.seen[id]
}
