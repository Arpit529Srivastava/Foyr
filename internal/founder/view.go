package founder

import (
	"fmt"
	"slices"
	"time"

	"github.com/Arpit529Srivastava/Foyr/internal/store"
)

// ist is the founder's time zone, for the activity grid; a fixed offset needs no tzdata in the image.
var ist = time.FixedZone("IST", 5*3600+1800)

// recentLimit is how many deployments the overview reads.
const recentLimit = 100

// Overview is everything the home page draws, worked out from real deployments.
type Overview struct {
	Median, Fastest time.Duration
	Total           int
	Capped          bool // the list hit its limit, so the numbers cover only the newest deployments
	LiveApps        int
	SuccessPct      int
	Bars            []Bar
	Days            []string
	Blocks          []string
	Heat            [][]Cell // rows are 4-hour blocks, columns the last 7 days
}

// Bar is one deployment in the push-to-live chart.
type Bar struct {
	ID     int64
	Height int    // percent of the chart
	Kind   string // live, failed or running
	Title  string
}

// Cell is one square of the activity grid.
type Cell struct {
	Count int
	Level int // 0 to 4
	Title string
}

// StepBar places one build step on the deployment's timeline, in percent.
type StepBar struct {
	store.Step
	Left, Width float64
}

// summarize works out the home page numbers; recent is newest first.
func summarize(recent []store.AppDeployment, apps []store.App, now time.Time) Overview {
	o := Overview{Total: len(recent), Capped: len(recent) >= recentLimit}
	for _, a := range apps {
		if a.LiveDeployment != 0 {
			o.LiveApps++
		}
	}

	var times []time.Duration
	reached, finished := 0, 0
	for _, d := range recent {
		switch d.Status {
		case store.StatusLive, store.StatusSuperseded, store.StatusFailed, store.StatusCanceled:
			finished++
		}
		if d.LiveAt != nil {
			reached++
			times = append(times, d.LiveAt.Sub(d.CreatedAt))
		}
	}
	if finished > 0 {
		o.SuccessPct = reached * 100 / finished
	}
	if len(times) > 0 {
		sorted := slices.Clone(times)
		slices.Sort(sorted)
		o.Median, o.Fastest = sorted[len(sorted)/2], sorted[0]
	}

	// Chart: the newest 14, oldest first, scaled to the slowest one shown.
	shown := recent[:min(len(recent), 14)]
	var slowest time.Duration
	for _, d := range shown {
		if d.LiveAt != nil {
			slowest = max(slowest, d.LiveAt.Sub(d.CreatedAt))
		}
	}
	for i := len(shown) - 1; i >= 0; i-- {
		d := shown[i]
		b := Bar{ID: d.ID, Kind: "running", Height: 14, Title: fmt.Sprintf("#%d %s: %s", d.ID, d.Slug, d.Status)}
		switch {
		case d.LiveAt != nil:
			took := d.LiveAt.Sub(d.CreatedAt)
			b.Kind, b.Height = "live", 100
			if slowest > 0 {
				b.Height = max(4, int(took*100/slowest))
			}
			b.Title = fmt.Sprintf("#%d %s: live in %s", d.ID, d.Slug, fmtDur(took))
		case d.Status == store.StatusFailed || d.Status == store.StatusCanceled:
			b.Kind = "failed"
		}
		o.Bars = append(o.Bars, b)
	}

	// Activity grid: last 7 days in IST, six 4-hour blocks.
	today := now.In(ist)
	start := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, ist).AddDate(0, 0, -6)
	for i := range 7 {
		o.Days = append(o.Days, start.AddDate(0, 0, i).Format("Mon"))
	}
	o.Blocks = []string{"0-4", "4-8", "8-12", "12-16", "16-20", "20-24"}
	o.Heat = make([][]Cell, 6)
	for r := range o.Heat {
		o.Heat[r] = make([]Cell, 7)
	}
	for _, d := range recent {
		t := d.CreatedAt.In(ist)
		day := int(time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, ist).Sub(start).Hours() / 24)
		if day < 0 || day > 6 {
			continue
		}
		o.Heat[t.Hour()/4][day].Count++
	}
	for r, row := range o.Heat {
		for c := range row {
			n := o.Heat[r][c].Count
			o.Heat[r][c].Level = min(n, 4)
			o.Heat[r][c].Title = fmt.Sprintf("%s %s IST: %d deployments", o.Days[c], o.Blocks[r], n)
		}
	}
	return o
}

// stepBars lays steps on one timeline from the first start to the last finish (or now, while running).
func stepBars(steps []store.Step, now time.Time) []StepBar {
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
		last = maxTime(last, end)
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

func maxTime(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}
