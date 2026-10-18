// Package ui holds formatting and layout helpers shared by foyrd's HTML pages.
package ui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Arpit529Srivastava/Foyr/internal/store"
)

// Dur prints 1m52s, 850ms or 3h04m.
func Dur(d time.Duration) string {
	switch {
	case d <= 0:
		return "-"
	case d < time.Second:
		return strconv.FormatInt(d.Milliseconds(), 10) + "ms"
	case d < time.Hour:
		return strings.Replace(d.Round(time.Second).String(), "m0s", "m", 1)
	default:
		d = d.Round(time.Minute)
		return fmt.Sprintf("%dh%02dm", int(d.Hours()), int(d.Minutes())%60)
	}
}

// Between is the time from a to b, or 0 when b is missing.
func Between(a time.Time, b *time.Time) time.Duration {
	if b == nil {
		return 0
	}
	return b.Sub(a)
}

// Since prints how long ago t was relative to now.
func Since(now, t time.Time) string {
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return strconv.Itoa(int(d.Minutes())) + "m ago"
	case d < 48*time.Hour:
		return strconv.Itoa(int(d.Hours())) + "h ago"
	default:
		return strconv.Itoa(int(d.Hours()/24)) + "d ago"
	}
}

// FirstLine is s up to its first newline.
func FirstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}

// Active reports whether a deployment's status is still changing.
func Active(status string) bool {
	switch status {
	case store.StatusQueued, store.StatusBuilding, store.StatusBuilt, store.StatusDeploying:
		return true
	}
	return false
}

// StepBar places one build step on the deployment's timeline, in percent.
type StepBar struct {
	store.Step
	Left, Width float64
}

// StepBars lays steps on one timeline from the first start to the last finish (or now, while running).
func StepBars(steps []store.Step, now time.Time) []StepBar {
	var first, last time.Time
	for _, s := range steps {
		if s.StartedAt == nil {
			continue
		}
		end := now
		if s.FinishedAt != nil {
			end = *s.FinishedAt
		}
		if first.IsZero() || s.StartedAt.Before(first) {
			first = *s.StartedAt
		}
		if end.After(last) {
			last = end
		}
	}
	span := last.Sub(first)
	bars := make([]StepBar, len(steps))
	for i, s := range steps {
		bars[i].Step = s
		if s.StartedAt == nil || span <= 0 {
			continue
		}
		end := now
		if s.FinishedAt != nil {
			end = *s.FinishedAt
		}
		bars[i].Left = pct(s.StartedAt.Sub(first), span)
		bars[i].Width = max(pct(end.Sub(*s.StartedAt), span), 0.6)
		if end.Equal(*s.StartedAt) {
			bars[i].Width = 0.6
		}
	}
	return bars
}

func pct(d, of time.Duration) float64 { return float64(int(float64(d)/float64(of)*1000)) / 10 }
