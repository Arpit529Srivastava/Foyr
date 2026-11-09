package db_test

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
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
	files, _ := filepath.Glob("migrations/*.sql")
	var want []string
	for _, f := range files {
		want = append(want, strings.TrimSuffix(filepath.Base(f), ".sql"))
	}
	if len(want) < 3 || !slices.Equal(applied, want) {
		t.Fatalf("applied = %v, want every migration in order: %v", applied, want)
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

func TestMigrateAuthTables(t *testing.T) {
	pool := dbtest.Empty(t)
	ctx := context.Background()

	// Apply 001 to 004 by hand so an app can exist before 005.
	if _, err := pool.Exec(ctx, `CREATE TABLE schema_migrations (
		version text PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		t.Fatal(err)
	}
	for _, v := range []string{"001_init", "002_builds", "003_runs", "004_slugs"} {
		sql, err := os.ReadFile("migrations/" + v + ".sql")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, string(sql)); err != nil {
			t.Fatalf("%s: %v", v, err)
		}
		if _, err := pool.Exec(ctx, "INSERT INTO schema_migrations (version) VALUES ($1)", v); err != nil {
			t.Fatal(err)
		}
	}
	mustExec := func(q string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	mustExec(`INSERT INTO installations (id, account_login) VALUES (1, 'octo')`)
	mustExec(`INSERT INTO apps (installation_id, github_repo_id, full_name) VALUES (1, 10, 'octo/old-app')`)

	if _, err := db.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	mustExec(`INSERT INTO apps (installation_id, github_repo_id, full_name) VALUES (1, 11, 'octo/new-app')`)

	for repo, want := range map[int64]string{10: "public", 11: "private"} {
		var mode string
		if err := pool.QueryRow(ctx, "SELECT access_mode FROM apps WHERE github_repo_id = $1", repo).Scan(&mode); err != nil {
			t.Fatal(err)
		}
		if mode != want {
			t.Errorf("repo %d access_mode = %q, want %q", repo, mode, want)
		}
	}

	if _, err := pool.Exec(ctx, "INSERT INTO users (email) VALUES ('A@b.c')"); err == nil {
		t.Error("an upper-case email was accepted, want the lower-case check to fail")
	}
	if _, err := pool.Exec(ctx, "UPDATE apps SET access_mode = 'secret' WHERE github_repo_id = 10"); err == nil {
		t.Error("access_mode 'secret' was accepted")
	}

	mustExec(`INSERT INTO users (email) VALUES ('a@b.c')`)
	if _, err := pool.Exec(ctx, `INSERT INTO app_members (app_id, user_id, installation_id, role)
		SELECT a.id, u.id, 1, 'god' FROM apps a, users u WHERE a.github_repo_id = 10`); err == nil {
		t.Error("role 'god' was accepted")
	}
	// An open invite is unique per app and email; a revoked one frees the slot.
	invite := `INSERT INTO invites (app_id, installation_id, email, role, invited_by)
		SELECT id, 1, 'x@y.z', 'member', 'founder' FROM apps WHERE github_repo_id = 10`
	mustExec(invite)
	if _, err := pool.Exec(ctx, invite); err == nil {
		t.Error("a second open invite for the same app and email was accepted")
	}
	mustExec(`UPDATE invites SET revoked_at = now()`)
	mustExec(invite)

	for _, table := range []string{"users", "app_members", "invites", "login_tokens", "auth_codes", "sessions"} {
		var ok bool
		if err := pool.QueryRow(ctx, "SELECT to_regclass($1) IS NOT NULL", table).Scan(&ok); err != nil || !ok {
			t.Errorf("table %s missing (err %v)", table, err)
		}
	}
}

func TestMigratePublicIDs(t *testing.T) {
	pool := dbtest.Empty(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx, `CREATE TABLE schema_migrations (
		version text PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		t.Fatal(err)
	}
	for _, v := range []string{"001_init", "002_builds", "003_runs", "004_slugs", "005_auth", "006_dashboard_login"} {
		sql, err := os.ReadFile("migrations/" + v + ".sql")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, string(sql)); err != nil {
			t.Fatalf("%s: %v", v, err)
		}
		if _, err := pool.Exec(ctx, "INSERT INTO schema_migrations (version) VALUES ($1)", v); err != nil {
			t.Fatal(err)
		}
	}
	mustExec := func(q string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	for _, e := range []string{"a@x.io", "b@x.io", "c@x.io"} {
		mustExec(`INSERT INTO users (email) VALUES ($1)`, e)
	}

	if _, err := db.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	mustExec(`INSERT INTO users (email) VALUES ('d@x.io')`)

	rows, err := pool.Query(ctx, `SELECT public_id FROM users`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	format := regexp.MustCompile(`^usr_[0-9a-f]{16}$`)
	seen := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		if !format.MatchString(id) {
			t.Errorf("public_id %q does not match the format", id)
		}
		seen[id] = true
	}
	rows.Close()
	if len(seen) != 4 {
		t.Errorf("got %d distinct public ids, want 4", len(seen))
	}

	if _, err := pool.Exec(ctx, `UPDATE users SET public_id = 'usr_XYZ' WHERE email = 'a@x.io'`); err == nil {
		t.Error("a malformed public_id was accepted")
	}
	mustExec(`UPDATE users SET public_id = 'usr_0123456789abcdef' WHERE email = 'a@x.io'`)
	if _, err := pool.Exec(ctx, `UPDATE users SET public_id = 'usr_0123456789abcdef' WHERE email = 'b@x.io'`); err == nil {
		t.Error("a duplicate public_id was accepted")
	}
	if _, err := pool.Exec(ctx, `UPDATE users SET public_id = NULL WHERE email = 'a@x.io'`); err == nil {
		t.Error("a NULL public_id was accepted")
	}
}
