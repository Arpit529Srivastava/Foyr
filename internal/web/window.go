package web

import (
	"sync"
	"time"
)

// maxTracked caps the keys a Window remembers, so a flood can't exhaust memory.
const maxTracked = 10000

// Window counts events per key over a sliding duration. An event is counted when Allow is called.
type Window struct {
	mu   sync.Mutex
	hits map[string][]time.Time
}

// Allow records an event for key and reports whether it is within limit events per span.
func (w *Window) Allow(key string, limit int, span time.Duration, now time.Time) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.hits == nil {
		w.hits = map[string][]time.Time{}
	}
	cut := now.Add(-span)
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
