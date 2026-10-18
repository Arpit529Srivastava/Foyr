package ui

import (
	"testing"
	"time"

	"github.com/Arpit529Srivastava/Foyr/internal/store"
)

func TestFmtDur(t *testing.T) {
	for d, want := range map[time.Duration]string{0: "-", 850 * time.Millisecond: "850ms", 50 * time.Second: "50s",
		112 * time.Second: "1m52s", 120 * time.Second: "2m", 3*time.Hour + 4*time.Minute: "3h04m"} {
		if got := Dur(d); got != want {
			t.Errorf("Dur(%v) = %q, want %q", d, got, want)
		}
	}
}

func TestStepBars(t *testing.T) {
	t0 := time.Date(2026, 10, 19, 10, 0, 0, 0, time.UTC)
	at := func(s int) *time.Time { v := t0.Add(time.Duration(s) * time.Second); return &v }
	bars := StepBars([]store.Step{
		{Name: "Clone", Status: store.StepSucceeded, StartedAt: at(0), FinishedAt: at(25)},
		{Name: "Build image", Status: store.StepSucceeded, StartedAt: at(25), FinishedAt: at(75)},
		{Name: "Health check", Status: store.StepRunning, StartedAt: at(75)},
		{Name: "Later", Status: store.StepPending},
	}, t0.Add(100*time.Second))
	if len(bars) != 4 {
		t.Fatalf("bars = %+v", bars)
	}
	if bars[0].Left != 0 || bars[0].Width != 25 || bars[1].Left != 25 || bars[1].Width != 50 {
		t.Errorf("finished steps = %+v %+v", bars[0], bars[1])
	}
	if bars[2].Left != 75 || bars[2].Width != 25 {
		t.Errorf("running step runs to now: %+v", bars[2])
	}
	if bars[3].Width != 0 {
		t.Errorf("pending step has no bar: %+v", bars[3])
	}
}
