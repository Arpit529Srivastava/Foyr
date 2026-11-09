# Customer dashboard sign-in: plan

Status: draft for the founder's approval, 2026-11-09. Builds on ADR-034 (dashboard) and ADR-035 (magic-link login).

## Goal

A developer you onboard opens `app.foyr.online`, enters their email, clicks the link, and sees only their own
GitHub account's apps. They can manage who can open each app (the Access tab). The founder still sees everything.

## Where things stand

- The dashboard works but every page is behind the founder key (`Allowed = fh.SignedIn`, `cmd/foyrd/server.go`).
- Login exists only for visitors of one app: tokens, codes and sessions all carry an `app_id`, and the session is
  a cookie on that app's host. It cannot be reused for the dashboard, and it should not be (an app visitor must
  never become a dashboard user).
- Nothing says which email belongs to which GitHub account (installation). That link is the missing piece.
- `app.foyr.online` already reaches foyrd (ADR-035) but `/` answers 404.

## Decisions (defaults, say so if you want a different one)

1. **Separate dashboard login, same pattern.** Magic link by email, Continue button that POSTs (mail scanners),
   single-use 15 minute token, SHA-256 only in the database, same rate limits, same "never say who is a member"
   answer. New tables, so no app-login code or table changes.
2. **Opaque session, not a signed token.** A random 32-byte value in the cookie `__Host-foyr_dash`, only its SHA-256 in the
   database. Every request checks the row, so logout and removing someone from an account take effect at once.
   7 days, like app sessions. (`Secure` is dropped only for a `http://localhost` dev setup, like the app login.)
3. **Account link table.** `account_members(installation_id, user_id, role)`. For now only the founder creates links,
   by hand on the founder page. GitHub sign-in (ADR-023) can create them automatically later.
4. **Roles:** `owner` and `admin` may change things (invites, access mode); `member` is read-only. The role is
   checked on every post, not just hidden in the UI.
5. **Audit trail names the person.** Changes made by a customer are logged with their email, not `founder`.

## The one rule that matters

Today `load()` finds an account among all installations. After this work an account is visible only if the
signed-in person is linked to it (founder sees all). That check lives in one place, `load()`, and every page and
every post goes through it. Every handler gets a test that a second account's ids answer 404 for a customer.

## Steps (three PRs, in this order)

The order is deliberate: the boundary lands before anyone can sign in with it.

**PR 1: tables and store** (`internal/db/migrations/006_dashboard_login.sql`, `internal/store`)
- `account_members`, `dashboard_login_tokens`, `dashboard_sessions`. Every row carries `installation_id` where it
  has one (ADR-026). `users` is reused.
- Store: link and unlink an email to an account (audited in the same transaction), start login (creates a token only
  if the email is linked to at least one account), finish login, create and look up and revoke a session, list a
  user's accounts, expire old rows.
- Tests: token single use and expiry, a stranger gets no token, unlinking kills open sessions, audit rows written.

**PR 2: who is asking** (`internal/dashboard`, `cmd/foyrd/server.go`)
- Replace `Allowed func(*http.Request) bool` with `Who func(*http.Request) (Person, bool)`. `Person` is
  founder (sees all) or customer (user id, email, accounts, roles). Wire it to the founder sign-in only, so nothing
  changes for users yet.
- `load()` and the account switcher use only the person's accounts. The "you are seeing this as the founder" banner
  shows for the founder only. Posts check the role. The audit actor comes from the person.
- Tests: with a fake customer person, every GET and POST path for another account's app answers 404; a read-only
  member gets 403 on posts; the switcher lists only their accounts; the founder is unchanged.

**PR 3: sign-in pages and founder linking** (`internal/dashboard`, `internal/founder`, `internal/mail`)
- `GET/POST /dashboard/login`, `GET/POST /dashboard/login/verify`, `POST /dashboard/logout`. A new sign-in email in
  the waitlist design. `/` on the app host redirects to `/dashboard`.
- Founder page: "People on this account" form (add or remove an email, pick a role).
- Tests: scanner-safe verify, identical answers for member and stranger, rate limits, return path safety (reuse
  `authn.SafeReturn`), logout clears the cookie and revokes the row, a removed person is signed out at once.
- Live check after merge: link your own second email, sign in on `app.foyr.online`, see only that account, invite
  someone from the Access tab.

## Security review focus

- A customer session must never open the founder page, and an app visitor session must never open the dashboard.
  Different cookie name, different table, different lookup. Test both directions.
- Cross-account reads and writes (listed above), including revoke and remove by id.
- Login form does not reveal who has an account (same page, status and timing as for strangers; email sent in the
  background).
- Same-origin posts only, small bodies, nonce CSP and the other dashboard headers are unchanged.

## Not in this plan

GitHub sign-in, self-serve signup, linking by anything but the founder, API tokens, billing. No AWS changes are
needed, so no `tofu apply`; foyrd redeploys on merge.

## Open questions

- Which email do you want to test with first (a second address of yours is enough)?
- Should a read-only `member` role exist at all for the first users, or is everyone you onboard an `owner`? The
  default above keeps the role because removing it later is harder than not using it.
