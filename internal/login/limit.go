package login

import (
	"sync"
	"time"
)

// Limits per 15 minutes: emails per address, and form posts per client IP (ADR-035).
const (
	RateWindow          = 15 * time.Minute
	MaxEmailsPerAddress = 3
	MaxPostsPerIP       = 10
	maxTracked          = 10000 // keys remembered at once, so a flood can't exhaust memory
)

// window counts events per key over a sliding RateWindow. An event is counted when allow is called.
type window struct {
	mu   sync.Mutex
	hits map[string][]time.Time
}

// allow records an event for key and reports whether it is within limit.
func (w *window) allow(key string, limit int, now time.Time) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.hits == nil {
		w.hits = map[string][]time.Time{}
	}
	cut := now.Add(-RateWindow)
	ts := w.hits[key]
	i := 0
	for i < len(ts) && !ts[i].After(cut) {
		i++
	}
	ts = ts[i:]
	if len(ts) >= limit {
		w.hits[key] = ts
		return false
	}
	if _, ok := w.hits[key]; !ok && len(w.hits) >= maxTracked {
		for k := range w.hits { // drop an arbitrary entry
			delete(w.hits, k)
			break
		}
	}
	w.hits[key] = append(ts, now)
	return true
}
