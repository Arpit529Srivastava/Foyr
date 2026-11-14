# Dashboard-to-app single sign-on: design

**Date:** 2026-11-14 · **Status:** Accepted (see ADR-039) · **Size:** small change (one new endpoint, two queries, one snapshot change)
**Builds on:** ADR-035 (app login), ADR-036 (dashboard sign-in), ADR-038 (identity contract) · **Becomes:** ADR-039

## Intent and why
A person who signs in to the dashboard and clicks one of their account's apps is asked to sign in again, and when they type the same email nothing arrives, because the app's own member list does not contain them. The founder hit this with foyr-hello: the dashboard knew the email, the app did not. Two separate lists with no link is correct for isolation and wrong for the team that owns the account.

The goal: an **owner or admin** of an account signs in once, opens any of that account's apps, and is in. No second email.

## Goals
- Owner or admin of the account that owns an app reaches that app with no second login, when they already have a dashboard session.
- They arrive with the same `usr_` id and email the dashboard knows, and an app role equal to their account role.
- Removing them from the account (or suspending the account) ends their app access within one authproxy poll (about 3 s).
- A person who cannot use SSO falls through to today's email login unchanged, with no loop.

## Non-goals
- Dashboard role `member` (read-only) does not get automatic app access. They still use the email login if invited to the app. (Founder's call, 2026-11-14.)
- GitHub sign-in, custom domains, the `@foyr/auth` SDK, API tokens.
- Founder-key sessions. The founder is not a user; SSO does not apply to them.
- Changing public apps or the email flow for people invited to one app only.

## Anti-goals (a failure even if it "works")
- An app session ever opening the dashboard. SSO is one-way: dashboard session to app code, never the reverse (ADR-036 stays true).
- Widening or sharing a cookie across hosts. `__Host-foyr_dash` stays host-only on the dashboard host (ADR-015).
- Account enumeration. The SSO endpoint answers only the person holding a valid dashboard session; the email form still gives byte-identical answers.
- A redirect loop between hosts.

## How other platforms do it (research, 2026-11-14)
Vercel, Cloudflare Access, Netlify and Google IAP all keep one central login and hand each app host its own cookie through a redirect, because browsers will not share a cookie across domains. Vercel lets signed-in team members in without a second prompt and sends everyone else to a request-access page. Cloudflare keeps a global session plus a per-app token and warns that multi-hop redirects can loop. Details that were not in official docs (Vercel's `sso-api` hop, Cloudflare's full redirect sequence) were not relied on.

## Approaches considered
1. **Handoff through the dashboard host (chosen).** `auth.` login page redirects to a new endpoint on the dashboard host, which can read `__Host-foyr_dash`, check the account, issue the existing one-time app code, and redirect to the app callback. Matches the industry pattern; no new cookie scope; reuses `ExchangeCode`.
2. **Auto-add `app_members` rows when someone joins an account.** Small, but the second login email remains, rows can drift from the account list, and removal needs extra cleanup. Rejected: does not fix the complaint.
3. **Shared cookie on `.foyr.online`.** One cookie everywhere. Rejected: ADR-015 and the `__Host-` rules forbid it, and it is the cookie-tossing risk the separate apps domain exists to avoid.

## Design

### Flow
1. Visitor opens `<slug>.<APPS_DOMAIN>/…` with no app session. authproxy redirects to `auth.<PLATFORM_DOMAIN>/login?app=<slug>&return=<path>` (unchanged).
2. **New:** on `GET /login` with a valid `app` and without `sso=0`, the login handler redirects to `<dash host>/dashboard/sso?app=<slug>&return=<path>`. (In development both hosts are `localhost:<port>`, so this is a same-host redirect.)
3. **New endpoint** `GET /dashboard/sso`, served only on the dashboard host:
   - read `__Host-foyr_dash`, resolve the person (`DashboardUserBySession`);
   - look up the app by `app` slug; require the person to be `owner` or `admin` in the app's account and the installation not removed or suspended;
   - on success: insert an `auth_codes` row (60 s, single use, hashed) for that user and app, write audit `user.signed_in` with detail `method: sso`, and 303 to `<slug>.<APPS_DOMAIN>/__foyr/callback?code=…&return=…` (same redirect shape as `login.verify`);
   - no dashboard session: 303 to `auth.…/login?app=<slug>&return=<path>&sso=0`, which shows today's email form;
   - valid session but not owner/admin of the app's account (member, other account, unknown app, suspended): a short "No automatic access" page, the same for every such case so it reveals nothing: "You're signed in as <email>. Only owners and admins of this app's account open it automatically. Ask an owner or admin, or use an email link if you were invited to this app." with a button to the same `…/login?…&sso=0` email form (Vercel-style request-access page instead of silence).
4. authproxy swaps the code for the app session exactly as today.

### Membership the proxy sees
authproxy only admits users in the app's `Members` map (`/internal/access`). `Store.Access` gains a union: for each live app, add every user who is `owner` or `admin` in `account_members` for the app's `installation_id`, with that role. An explicit `app_members` row for the same user wins. No rows are copied, so removing a person from the account, or suspending/removing the installation, drops them from the snapshot on the next poll. The proxy's existing check then refuses them, and the existing revoked-session list needs no change.

### Safety rules
- `return` goes through `authn.SafeReturn`; `app` through `cleanSlug`, both as today. The code is only ever sent to `<slug>.<APPS_DOMAIN>` built server-side.
- The endpoint is a GET that issues a single-use 60-second code to the app host, as the existing verify redirect does. It does not set or change any cookie.
- Each SSO hop stores a code and an audit row, so it is rate limited per person (20 per 15 minutes, as the email flow uses a 15 minute window); over the limit goes to the email form and mints nothing (review finding).
- `sso=0` is read only by the login handler and only to skip the hop, so a failed handoff cannot bounce back to itself.
- The dashboard cookie is read on the dashboard host only; the app session cookie is never accepted there.

### Errors people see
- No dashboard session: the email form, as today.
- Dashboard session but not owner/admin: the "No automatic access" page above, with a way on to the email form.
- Signed in to the app but not a member: authproxy's existing "No access" page, unchanged.
- foyrd unreachable: authproxy behaviour unchanged.

## Parts of the how decided (and by whom)
- Owner and admin only: **founder**.
- Handoff through the dashboard host, access derived live in the snapshot instead of copying rows, role carried over one-to-one: **my call**, from the research and ADR-015/036.
- Hop starts at the auth host's `/login`, not in authproxy: **my call**. It needs no new authproxy setting, so no infra change or `tofu apply`. Cost is one extra redirect.

## Tests (first, per CLAUDE.md)
- store: owner and admin get a code; member, stranger, removed or suspended installation, unknown app get none; code is single use and expires; audit row written.
- store `Access`: owner/admin appear in `Members` with their role; explicit row wins; unlinking drops them; suspended installation drops them.
- login handler: `GET /login` redirects to the SSO endpoint unless `sso=0` or `app` invalid.
- dashboard handler: success redirect target and query; no session lands on `/login…&sso=0`; non-owner/admin gets the same "No automatic access" page for member, other account, unknown app and suspended; endpoint 404s on other hosts.
- proxy: a user who is owner/admin by account is admitted with the right role; after removal is refused after one poll.
- loop test: a failed SSO ends on the email form in one hop.

## Left to the builder
File and function names; where the SSO handler is registered (likely `internal/dashboard`, which owns the host check); how `DashURL` is derived for the redirect; whether the union lives in SQL or Go.

## Rollout
Additive: no migration, no new secret, no authproxy or infra change. Ship foyrd only. Deploys on merge to `main` through a PR (ADR-017); then check live that foyr-hello opens from the dashboard with no email.
