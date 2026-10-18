package founder

import (
	"fmt"
	"slices"
	"time"

	"github.com/Arpit529Srivastava/Foyr/internal/store"
	"github.com/Arpit529Srivastava/Foyr/internal/ui"
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

// Shared helpers, under the names the founder templates and tests already use.
var (
	fmtDur    = ui.Dur
	between   = ui.Between
	since     = ui.Since
	firstLine = ui.FirstLine
	active    = ui.Active
	stepBars  = ui.StepBars
)

// StepBar is one step on a deployment's timeline.
type StepBar = ui.StepBar
