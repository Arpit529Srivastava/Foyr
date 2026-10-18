package founder

import (
	"testing"
	"time"

	"github.com/Arpit529Srivastava/Foyr/internal/store"
)

func dep(id int64, status string, created time.Time, toLive time.Duration) store.AppDeployment {
	d := store.AppDeployment{Deployment: store.Deployment{ID: id, Status: status, CreatedAt: created}, Slug: "app"}
	if toLive > 0 {
		live := created.Add(toLive)
		d.LiveAt = &live
	}
	return d
}

func TestSummarize(t *testing.T) {
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	recent := []store.AppDeployment{ // newest first, as the store returns them
		dep(5, store.StatusLive, now.Add(-time.Hour), 112*time.Second),
		dep(4, store.StatusSuperseded, now.Add(-25*time.Hour), 130*time.Second),
		dep(3, store.StatusFailed, now.Add(-26*time.Hour), 0),
		dep(2, store.StatusDeploying, now.Add(-2*time.Minute), 0),
		dep(1, store.StatusSuperseded, now.Add(-30*24*time.Hour), 200*time.Second),
	}
	apps := []store.App{{LiveDeployment: 5}, {}}
	o := summarize(recent, apps, now)

	if o.Median != 130*time.Second || o.Fastest != 112*time.Second {
		t.Errorf("median %v fastest %v", o.Median, o.Fastest)
	}
	if o.Total != 5 || o.LiveApps != 1 || o.SuccessPct != 75 {
		t.Errorf("total %d live apps %d success %d%%", o.Total, o.LiveApps, o.SuccessPct)
	}
	// Bars run oldest to newest; the slowest live one fills the chart.
	if len(o.Bars) != 5 || o.Bars[0].ID != 1 || o.Bars[4].ID != 5 || o.Bars[0].Height != 100 || o.Bars[4].Height != 56 {
		t.Fatalf("bars = %+v", o.Bars)
	}
	if o.Bars[1].Kind != "running" || o.Bars[2].Kind != "failed" || o.Bars[3].Kind != "live" {
		t.Errorf("kinds = %s %s %s", o.Bars[1].Kind, o.Bars[2].Kind, o.Bars[3].Kind)
	}

	// Heat map: 7 day columns ending today (IST), 6 four-hour rows; the 30-day-old one is outside it.
	if len(o.Days) != 7 || len(o.Heat) != 6 || len(o.Heat[0]) != 7 {
		t.Fatalf("heat shape %d x %d", len(o.Heat), len(o.Days))
	}
	count := 0
	for _, row := range o.Heat {
		for _, c := range row {
			count += c.Count
		}
	}
	if count != 4 {
		t.Errorf("heat counts %d deployments, want 4", count)
	}
	// #5 (16:30 IST) and #2 (17:28 IST) share today's 16-20 block.
	if c := o.Heat[4][6]; c.Count != 2 || c.Level != 2 {
		t.Errorf("today 16-20 IST = %+v", c)
	}
}

func TestSummarizeEmpty(t *testing.T) {
	o := summarize(nil, nil, time.Now())
	if o.Total != 0 || o.Median != 0 || len(o.Bars) != 0 || o.SuccessPct != 0 {
		t.Fatalf("empty = %+v", o)
	}
}

func TestSummarizeSurvivesInstantLive(t *testing.T) {
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	d := dep(1, store.StatusLive, now, 0)
	d.LiveAt = &now // live at the same instant it was created
	o := summarize([]store.AppDeployment{d}, nil, now)
	if len(o.Bars) != 1 || o.Bars[0].Height <= 0 {
		t.Fatalf("bars = %+v", o.Bars)
	}
}

func TestSummarizeSaysWhenItIsCapped(t *testing.T) {
	now := time.Now()
	var many []store.AppDeployment
	for i := range 100 {
		many = append(many, dep(int64(100-i), store.StatusLive, now.Add(-time.Duration(i)*time.Hour), time.Minute))
	}
	if o := summarize(many, nil, now); !o.Capped || summarize(many[:5], nil, now).Capped {
		t.Fatal("Capped must be set exactly when the list hit the limit")
	}
}
