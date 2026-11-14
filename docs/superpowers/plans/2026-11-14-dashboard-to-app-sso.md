# Dashboard-to-app SSO Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** An owner or admin who is signed in to the dashboard opens any of that account's private apps with no second sign-in.

**Architecture:** The app login page (`auth.` host) hops to a new `/dashboard/sso` endpoint on the dashboard host, the only host that can read `__Host-foyr_dash`. That endpoint checks the person's role in the app's account and issues the existing one-time app code; authproxy swaps it for the app session as today. `Store.Access` adds account owners and admins to each app's member map so the proxy admits them, computed live with no copied rows.

**Tech Stack:** Go (foyrd), Postgres via pgx, Go html templates. No migration, no new secret, no authproxy or infra change.

**Spec:** `docs/superpowers/specs/2026-11-14-dashboard-to-app-sso-design.md`

## Global Constraints

- Owner and admin only. Dashboard role `member` gets no automatic app access (spec, non-goals).
- SSO is one way: dashboard session to app code. An app session never opens the dashboard (ADR-036).
- `__Host-foyr_dash` stays host-only; no cookie scope changes (ADR-015).
- One-time app code: 60 s (`store.AuthCodeTTL`), single use, stored hashed.
- `return` always goes through `authn.SafeReturn`; the app slug always through the slug pattern `^[a-z0-9][a-z0-9-]{0,38}[a-z0-9]$`; the code is only sent to `<slug>.<APPS_DOMAIN>` built server-side.
- `sso=0` on `/login` means "skip the hop"; nothing else reads it. A failed handoff ends on the email form in one hop.
- Audit every sign-in: `user.signed_in` with detail `method: sso`, actor `foyr_user`.
- Repo rules (CLAUDE.md): work on a branch, never push `main`; commit with `-s`; tests first; `gofmt -l . && go vet ./cmd/... ./internal/... && go test ./cmd/... ./internal/...` must pass; DB tests need `FOYR_TEST_DATABASE_URL` (docs/dev/database.md) or they skip; no `tofu apply`; add an ADR instead of silently changing a decision.

## Review Focus

- Dashboard `member` (read-only) opens SSO: no code, the "No automatic access" page. Task 1 and 4 tests.
- Person removed from the account while their dashboard session is still alive (their last-account rule keeps the session if they have another account): no code on the next SSO, gone from the snapshot. Task 1 and 2 tests.
- Suspended or removed installation: no code, not in the snapshot. Task 1 and 2 tests.
- Hostile `app` or `return` (`app=evil.com/x`, `app=A..B`, `return=//evil.test`): no hop, no redirect to a foreign host, return becomes `/`. Task 3 and 4 tests.
- Loop: `/login?app=crm&sso=0` never redirects; SSO denied or signed-out ends on that URL. A founder-only cookie counts as signed out. Task 3 and 4 tests.

---

### Task 1: Issue the app code for an owner or admin

**Files:**
- Create: `internal/store/sso.go`
- Test: `internal/store/sso_test.go`

**Interfaces:**
- Consumes: `Store.tx`, `audit`, `authn.NewSecret`, `AuthCodeTTL`, `ErrNotFound` (all in `internal/store`); test helpers `newStore`, `seedApp`, `signedIn`, `founder`, `count`, `ctx`.
- Produces: `var ErrNoSSO = errors.New("no automatic access to this app")` and `func (s *Store) IssueSSOCode(ctx context.Context, userID int64, slug string, now time.Time) (code string, err error)`. Tasks 3 and 4 depend on these names.

- [ ] **Step 1: Write the failing tests in `internal/store/sso_test.go`**
  - `TestIssueSSOCode`: seed app crm in installation 1 and an app in installation 2. Link `owner@x.test` as owner and `admin@x.test` as admin of installation 1 with the `signedIn` helper (get user ids from `DashboardUserBySession`). Assert both get a non-empty code, and `ExchangeCode(ctx, code, crm.Slug, now)` returns a Session whose `UserID`, `Email` and `PublicID` match that user and `AppID == crm.ID`.
  - `TestIssueSSORefusals` (table): role `member`; owner of installation 2 asking for crm's slug; unknown slug; installation suspended; installation removed; user unlinked after sign-in (`UnlinkAccountMember`). Each returns `store.ErrNoSSO` and writes no `auth_codes` row (`count`).
  - `TestSSOCodeSingleUseAndExpiry`: second `ExchangeCode` of the same code returns `ErrInvalidToken`; a code older than `AuthCodeTTL` returns `ErrInvalidToken`.
  - `TestSSOAudit`: exactly one `audit_events` row with action `user.signed_in` and details containing `"method": "sso"` per successful call; none per refusal.
  - Pin that the raw code is not stored: `SELECT count(*) FROM auth_codes WHERE code_hash = $1` with the raw string bytes is 0 (copy the pattern of `TestTokensStoredHashed`).

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/store -run 'SSO' -v`
Expected: FAIL to compile (`IssueSSOCode`, `ErrNoSSO` undefined).

- [ ] **Step 3: Implement `IssueSSOCode` and `ErrNoSSO` in `internal/store/sso.go`**

In one transaction: select the live app by slug joined to its installation (`removed_at IS NULL` on both, installation `suspended_at IS NULL`) and to `account_members` for `userID` with role in `('owner','admin')`; no row means `ErrNoSSO`. Insert into `auth_codes (code_hash, user_id, app_id, installation_id, expires_at)` exactly as `FinishLogin` does, then `audit` an `AuditEvent` like `FinishLogin`'s with `Details: map[string]any{"method": "sso"}`. Return the raw code.

- [ ] **Step 4: Run to verify pass**

Run: `go test ./internal/store -run 'SSO' -v`
Expected: PASS (needs `FOYR_TEST_DATABASE_URL`; a skip is not a pass).

- [ ] **Step 5: Commit**

```bash
git add internal/store/sso.go internal/store/sso_test.go
git commit -s -m "feat(store): issue app code for dashboard owners and admins"
```

---

### Task 2: Let the proxy admit account owners and admins

**Files:**
- Modify: `internal/store/auth.go` (`Store.Access`, around line 316-368)
- Test: `internal/store/auth_test.go` (add beside `TestAccessSnapshot`)

**Interfaces:**
- Consumes: `authn.AppAccess.Members map[int64]string` (user id to role) as already filled from `app_members`.
- Produces: `Store.Access` now also lists, for every live app whose installation is live and not suspended, each `account_members` row with role `owner` or `admin` for that installation, role copied. An existing explicit `app_members` entry is never overwritten. No signature change.

- [ ] **Step 1: Write the failing test `TestAccessSnapshotIncludesAccountAdmins`**
  - seed crm (installation 1) and blog (installation 2). Link owner@ (owner), admin@ (admin), member@ (member) to installation 1. Assert crm's `Members` has owner and admin with roles `owner` and `admin`, not member, and blog's `Members` has none of them.
  - Invite ana as `viewer` to crm through `invitedLogin`, then link ana as owner of installation 1: `Members[ana]` stays `viewer` (explicit wins).
  - `UnlinkAccountMember` for admin@ then `Access` again: admin@ gone. Suspend installation 1: nobody from the account is listed.

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/store -run TestAccessSnapshotIncludesAccountAdmins -v`
Expected: FAIL (owner and admin missing from `Members`).

- [ ] **Step 3: Extend `Store.Access`**

After the `app_members` loop, run one more query joining `apps`, `installations` (live, not suspended) and `account_members` filtered to roles `owner`, `admin`; set `snap.Apps[byID[appID]].Members[userID] = role` only when the key is absent. Close rows and check `rows.Err()` like the neighbouring loops.

- [ ] **Step 4: Run to verify pass, and the old snapshot test still passes**

Run: `go test ./internal/store -run 'TestAccessSnapshot' -v`
Expected: PASS for both tests.

- [ ] **Step 5: Commit**

```bash
git add internal/store/auth.go internal/store/auth_test.go
git commit -s -m "feat(store): admit account owners and admins in the access snapshot"
```

---

### Task 3: Login page hops to the dashboard SSO endpoint

**Files:**
- Modify: `internal/authn/authn.go` (add `CleanSlug` next to `SafeReturn`), `internal/login/login.go` (`Handler.DashURL`, `form`, use `authn.CleanSlug` in place of the local `cleanSlug`)
- Test: `internal/authn/authn_test.go`, `internal/login/login_test.go`

**Interfaces:**
- Consumes: `authn.SafeReturn`.
- Produces: `func CleanSlug(s string) string` in `internal/authn` (returns s if it matches the slug pattern, else ""); `login.Handler.DashURL string` (empty turns the hop off). Task 4 uses `authn.CleanSlug`.

- [ ] **Step 1: Write the failing tests**
  - authn: `TestCleanSlug` table: `crm`, `foyr-hello` pass; `""`, `A..B`, `evil.com/x`, `-a`, `a-`, a 41-character slug return `""`.
  - login: add `const dashURL = "https://app.test"` and a rig built with `DashURL: dashURL` (leave `newRig` as is so existing tests keep passing; add a small `newSSORig`). `TestLoginHopsToDashboardSSO`: `GET /login?app=crm&return=/reports` on the auth host returns 303 with `Location == "https://app.test/dashboard/sso?app=crm&return=%2Freports"`.
  - `TestLoginSkipsHop`: `sso=0` returns 200 with the form; `app=Evil.com`, `app=a/b`, and no `app` return 200 with the form; a rig with empty `DashURL` returns 200. `return=//evil.test` redirects with `return=%2F`.

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/authn ./internal/login -run 'CleanSlug|Hop' -v`
Expected: FAIL (undefined `CleanSlug`, `DashURL`).

- [ ] **Step 3: Implement**
  - `authn.CleanSlug` using the regex currently in `login.go:92`; replace `login.cleanSlug` calls with it and delete the local copy.
  - In `Handler.form`: when `h.DashURL != ""`, the cleaned `app` is non-empty and `r.URL.Query().Get("sso") != "0"`, set `Cache-Control: no-store` and `http.Redirect(w, r, strings.TrimRight(h.DashURL, "/")+"/dashboard/sso?"+url.Values{"app": {app}, "return": {authn.SafeReturn(...)}}.Encode(), http.StatusSeeOther)`.

- [ ] **Step 4: Run to verify pass**

Run: `go test ./internal/authn ./internal/login -v`
Expected: PASS, including every existing login test.

- [ ] **Step 5: Commit**

```bash
git add internal/authn internal/login
git commit -s -m "feat(login): hop to dashboard SSO before the email form"
```

---

### Task 4: The `/dashboard/sso` endpoint and its no-access page

**Files:**
- Modify: `internal/dashboard/signin.go` (`Auth` interface gains a method, new handler), `internal/dashboard/dashboard.go` (`Handler.AuthURL` field, one route), `cmd/foyrd/server.go` (wire `login.Handler.DashURL` and `dash.AuthURL`)
- Create: `internal/dashboard/templates/sso_denied.html` (define `sso_denied.html` with the existing `sihead`/`sifoot` blocks, like `signin_error.html` in `templates/signin.html`)
- Test: `internal/dashboard/signin_test.go` (extend `fakeAuth`, add tests)

**Interfaces:**
- Consumes: `Handler.person`, `Handler.signinRoute`, `Handler.render`, `Handler.AppsDomain`, `Handler.AppsScheme`, `Handler.Auth`, `authn.CleanSlug`, `authn.SafeReturn`, `store.IssueSSOCode`, `store.ErrNoSSO`.
- Produces: `Auth.IssueSSOCode(ctx context.Context, userID int64, slug string, now time.Time) (string, error)` on the interface (the `*store.Store` already satisfies it after Task 1); `Handler.AuthURL string` (`https://auth.<PLATFORM_DOMAIN>`, localhost in development); route `GET /dashboard/sso` served only on the dashboard host.

- [ ] **Step 1: Write the failing tests (set `AuthURL: "https://auth.foyr.test"` and `AppsScheme: "https"` in the rig; give `fakeAuth` an `IssueSSOCode` that returns code `"sso-code"` when the user and slug are in an allowed set, else `store.ErrNoSSO`)**
  - `TestSSOIssuesCode`: cookie `ana-cookie` (allowed for `crm`), `GET /dashboard/sso?app=crm&return=/reports` on `dashHost` returns 303 to `https://crm.apps.test/__foyr/callback?code=sso-code&return=%2Freports`.
  - `TestSSOSignedOut`: no cookie, a bad cookie, and a founder-only cookie each return 303 to `https://auth.foyr.test/login?app=crm&return=%2Freports&sso=0` and never call `IssueSSOCode`.
  - `TestSSODenied`: a signed-in person not allowed for the slug (member role, other account, unknown app, suspended all map to `ErrNoSSO` in the fake) gets 403 and a page containing their email and the same `…&sso=0` link; the bodies for those four cases are byte-identical when the slug in the link is the same, so the page reveals nothing about the app.
  - `TestSSOHostileInput`: `app=evil.com/x` and `app=A..B` return 404; `return=//evil.test` becomes `return=%2F`; the redirect host is always `crm.apps.test`.
  - `TestSSOOtherHost`: any non-dashboard host returns 404. A store error other than `ErrNoSSO` returns 500 and sets no `Location`.

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/dashboard -run SSO -v`
Expected: FAIL to compile (`IssueSSOCode` not on `Auth`, no route).

- [ ] **Step 3: Implement `func (h *Handler) sso(w http.ResponseWriter, r *http.Request)` in `signin.go`**

Register `GET /dashboard/sso` through `signinRoute`. Steps: clean the slug (404 when empty); `ret := authn.SafeReturn(...)`; `p, ok := h.person(r)`; treat `!ok || p.Founder || p.UserID == 0` as signed out (303 to the email URL below); call `h.Auth.IssueSSOCode(r.Context(), p.UserID, slug, h.now())`; `store.ErrNoSSO` renders `sso_denied.html` with status 403 and data `Email`, `LoginURL`; other errors call `h.fail`. Success is a 303 to `AppsScheme://<slug>.<AppsDomain>/__foyr/callback?code=…&return=…`. The email URL is `strings.TrimRight(h.AuthURL, "/")+"/login?"+url.Values{"app", "return", "sso": {"0"}}.Encode()` (check that `Encode` order matches the test strings, or assert on parsed query). Page copy: "You're signed in as {{.Email}}. Only owners and admins of this app's account open it automatically. Ask an owner or admin, or use an email link if you were invited to this app." with the button as a link to `LoginURL`. Wire `AuthURL` and `login.Handler.DashURL: dashboardURL(cfg)` in `cmd/foyrd/server.go`.

- [ ] **Step 4: Run to verify pass, then the whole module**

Run: `go test ./internal/dashboard -v && gofmt -l . && go vet ./cmd/... ./internal/... && go test ./cmd/... ./internal/...`
Expected: PASS; `gofmt -l` prints nothing.

- [ ] **Step 5: Commit**

```bash
git add internal/dashboard cmd/foyrd/server.go
git commit -s -m "feat(dashboard): SSO endpoint for owners and admins"
```

---

### Task 5: Docs, local check, ship

**Files:**
- Modify: `docs/architecture/decisions.md` (append ADR-039), `docs/dev/login.md` (new "Dashboard to app SSO" section after the customer sign-in section)

**Interfaces:** none.

- [ ] **Step 1: Write ADR-039 "Dashboard-to-app SSO"** after ADR-038, Status Accepted, Builds on ADR-035/036/038, in the existing ADR layout. Decision bullets: flow, owner/admin only with the member exclusion, derived membership in the snapshot, `sso=0` loop guard, one-way only, `method: sso` audit. Why: one sign-in for the account's team, matching Vercel, Cloudflare Access and Netlify (per-app cookie via redirect handoff). Consequences: no migration or infra change; ships with foyrd only; removing someone from the account ends app access within one poll.
- [ ] **Step 2: Add the local steps to `docs/dev/login.md`:** run the three terminals as in the guide; at `/founder/accounts` link `you@example.com` as `owner`, sign out, sign in at `/dashboard/login` using the link printed in foyrd's log; open `http://foyr-hello.apps.localhost:8081/` in the same browser: it lands in the app with no emailed link and the echo app shows your `X-Foyr-*` headers. Then link a second address as `member` and confirm it sees the "No automatic access" page, and that removing the owner from the account sends the first browser back to sign in within about 3 seconds.
- [ ] **Step 3: Run that local check yourself** and note any difference from the doc. Expected: both browsers behave as written.
- [ ] **Step 4: Full checks**

Run: `gofmt -l . && go vet ./cmd/... ./internal/... && go test ./cmd/... ./internal/...`
Expected: no gofmt output, vet clean, all tests pass with `FOYR_TEST_DATABASE_URL` set.

- [ ] **Step 5: Ship with the `foyr-ship-change` skill** (branch such as `feat/dashboard-app-sso`, signed commit as the personal account, push over SSH, open a PR; `main` is protected). After merge and deploy, check live: signed in at `https://app.foyr.online/dashboard`, open `https://foyr-hello.apps.foyr.online/` and land in the app with no email (use `foyr-live-check` for the usual route checks).

```bash
git add docs/architecture/decisions.md docs/dev/login.md
git commit -s -m "docs: ADR-039 dashboard-to-app SSO and local steps"
```
