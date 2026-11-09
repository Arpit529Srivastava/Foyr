package web

import (
	"strconv"
	"testing"
	"time"
)

func TestWindowAllowsUpToTheLimitThenRecovers(t *testing.T) {
	var w Window
	now := time.Now()
	for i := range 3 {
		if !w.Allow("a", 3, time.Minute, now.Add(time.Duration(i)*time.Second)) {
			t.Fatalf("event %d refused", i)
		}
	}
	if w.Allow("a", 3, time.Minute, now.Add(10*time.Second)) {
		t.Error("fourth event within the window was allowed")
	}
	if !w.Allow("b", 3, time.Minute, now.Add(10*time.Second)) {
		t.Error("another key is affected")
	}
	if !w.Allow("a", 3, time.Minute, now.Add(2*time.Minute)) {
		t.Error("the window did not slide")
	}
}

func TestWindowRemembersABoundedNumberOfKeys(t *testing.T) {
	var w Window
	now := time.Now()
	for i := range maxTracked + 50 {
		w.Allow(strconv.Itoa(i), 1, time.Hour, now)
	}
	if len(w.hits) > maxTracked {
		t.Errorf("%d keys tracked, want at most %d", len(w.hits), maxTracked)
	}
}
