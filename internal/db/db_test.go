package db_test

import (
	"context"
	"slices"
	"sync"
	"testing"

	"github.com/Arpit529Srivastava/Foyr/internal/db"
	"github.com/Arpit529Srivastava/Foyr/internal/db/dbtest"
)

func TestMigrateAppliesOnce(t *testing.T) {
	pool := dbtest.Empty(t)
	ctx := context.Background()

	applied, err := db.Migrate(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(applied, []string{"001_init", "002_builds"}) {
		t.Fatalf("applied = %v, want 001_init then 002_builds", applied)
	}
	again, err := db.Migrate(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != 0 {
		t.Fatalf("second run applied %v, want nothing", again)
	}
	for _, table := range []string{"installations", "apps", "deployments", "deployment_steps",
		"deployment_events", "jobs", "webhook_deliveries", "audit_events"} {
		var ok bool
		if err := pool.QueryRow(ctx, "SELECT to_regclass($1) IS NOT NULL", table).Scan(&ok); err != nil || !ok {
			t.Errorf("table %s missing (err %v)", table, err)
		}
	}
}

func TestMigrateConcurrentCopiesApplyOnce(t *testing.T) {
	pool := dbtest.Empty(t)
	var (
		wg    sync.WaitGroup
		mu    sync.Mutex
		total int
	)
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			applied, err := db.Migrate(context.Background(), pool)
			if err != nil {
				t.Error(err)
			}
			mu.Lock()
			total += len(applied)
			mu.Unlock()
		}()
	}
	wg.Wait()
	var want int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM schema_migrations").Scan(&want); err != nil {
		t.Fatal(err)
	}
	if total != want || want < 2 {
		t.Fatalf("copies applied %d migrations in total, want each of the %d exactly once", total, want)
	}
}

func TestOpenRejectsBadURL(t *testing.T) {
	if _, err := db.Open(context.Background(), "postgres://%zz"); err == nil {
		t.Fatal("want error for malformed url")
	}
}
