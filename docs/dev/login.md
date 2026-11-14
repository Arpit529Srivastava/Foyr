# Magic-link login, locally (ADR-035)

Every app can sit behind login: a visitor enters an email, clicks a link, and the app receives
`X-Foyr-User-Id`, `X-Foyr-Email` and `X-Foyr-Role`. This guide runs the whole thing on your laptop.

```
browser ─► authproxy :8091 ──no session──► 302 foyrd :8090/login?app=<slug>
                                           email form ─► link in foyrd's LOG (no email is sent locally)
           link ─► "Continue" button (POST) ─► 303 <slug>.apps.localhost:8091/__foyr/callback?code=…
authproxy swaps the code at foyrd for a signed session, sets the cookie, and from then on
forwards to the app with the identity headers.
```

## Run it
Three terminals (or the three `.claude/launch.json` entries `foyrd-login`, `authproxy-login`, `echo-app`).
Use Chrome or Firefox: `*.localhost` resolves to your machine, and they accept the Secure session cookie on it.

```bash
python3 scripts/echo-app.py
```
```bash
FOYR_WORKERS=0 FOYR_FOUNDER_KEY=local-preview-key-not-a-secret-0001 scripts/foyrd-dev.sh
```
```bash
FOYR_STATIC_ROUTES=foyr-hello=localhost:3000 scripts/authproxy-dev.sh
```

The first start makes a throwaway key pair in `~/.config/foyr/dev-session.{key,pub}` (`go run ./cmd/sessionkey`,
because macOS's openssl cannot make Ed25519 keys). foyrd starts with login on, signs with the key, and logs emails
instead of sending them (`FOYR_AUTH_URL` is `http://localhost:8080`; set `FOYRD_PORT` if you moved foyrd).

## Try it
1. Open http://localhost:8080/founder, paste the founder key, open an app (sidebar, then **Access and members**).
2. Make it **private**, invite `you@example.com`.
3. Open http://foyr-hello.apps.localhost:8081/ in a private window: you are sent to the sign-in page.
4. Enter the invited address. The "email" is in foyrd's log: look for `email (not sent, log sender)` and copy the
   `http://localhost:8080/login/verify?…` link from its `text` field.
5. Open the link, press **Continue**. The echo app prints the request it got: your `X-Foyr-*` headers, and no session cookie.
6. On the founder page, **Remove** the member. Within about 3 seconds the same browser is sent back to sign in.

A stranger's address gets the same "check your email" page and no email, so the form never says who is a member.
Without login keys (`FOYR_SESSION_PUBLIC_KEY` unset) authproxy forwards everything as before.

## Ports and settings
| Setting | Local value |
|---|---|
| foyrd (`FOYR_ADDR`) / internal (`FOYR_INTERNAL_ADDR`) | `:8080` / `:9090` |
| authproxy (`AUTHPROXY_ADDR`) | `:8081` |
| foyrd `FOYR_APPS_DOMAIN` (with the proxy's port, for redirects) | `apps.localhost:8081` |
| authproxy `FOYR_APPS_DOMAIN` | `apps.localhost` |
| `FOYR_SESSION_KEY` (foyrd) / `FOYR_SESSION_PUBLIC_KEY` (authproxy) | from `dev-session.key` / `.pub` |
| `FOYR_FOYRD_URL` (authproxy) | `http://localhost:9090` |

## Tests
`go test ./cmd/... ./internal/...` with `FOYR_TEST_DATABASE_URL` set (docs/dev/database.md) covers every rule above:
tokens are single use and stored hashed, a GET never spends the link, open-redirect return paths become `/`,
visitor-sent `X-Foyr-*` headers and the session cookie never reach the app, and removed members are refused after one poll.

## The emails
The sign-in and invite emails use the same design as the waitlist emails (`site/lib/emails/layout.ts`), rebuilt in Go in
`internal/mail/layout.go`. To look at them in a browser (screenshots are in `docs/website/emails/`):
```bash
mkdir -p /tmp/previews && FOYR_EMAIL_PREVIEW_DIR=/tmp/previews go test -run TestWritePreviews ./internal/mail && open /tmp/previews/login.html /tmp/previews/invite.html
```
The banner image comes from the public site (`/email/banner.jpg`); without a public site (local development) the emails carry no image.

## Customer sign-in to the dashboard (ADR-036)

1. Run foyrd as above. Sign in at `http://localhost:8090/founder`, open **Accounts**, add an email to the account (owner), then **Sign out**.
2. Open `http://localhost:8090/dashboard`. It sends you to `/dashboard/login`. Enter the email; the sign-in link is printed in foyrd's log (`/dashboard/login/verify?t=...`). Open it and press **Continue**.
3. You see only that account, without the founder banner. **Sign out** is in the account menu.
4. The founder and customer cookies share `localhost`, so sign the founder out first, as in step 1.

## Dashboard to app SSO (ADR-039)

An owner or admin who is signed in to the dashboard opens their account's private apps with no second sign-in.
```
authproxy ─no app session─► auth host /login?app=<slug>
                              └─303► dashboard host /dashboard/sso?app=<slug>&return=…   (reads __Host-foyr_dash)
                                       owner/admin of the app's account ─303► <slug>.apps…/__foyr/callback?code=…
                                       signed out ─303► /login?app=<slug>&sso=0   (the email form)
                                       signed in, not owner/admin ─403 "No automatic access" page (link to the email form)
```
Locally the auth and dashboard hosts are both `localhost:8080`, so the hop is a redirect on the same host.

Try it, with the three terminals from above:
1. At `http://localhost:8080/founder/accounts`, link `you@example.com` to the account as **owner**, and `mia@example.com` as **member**. Sign the founder out.
2. Sign in at `http://localhost:8080/dashboard/login` as `you@example.com` (link in foyrd's log).
3. In the same browser open `http://foyr-hello.apps.localhost:8081/` (make the app private first). You land in the app with no email; the echo app shows your `X-Foyr-*` headers.
4. Sign in as `mia@example.com` instead and open the app: you get "No automatic access" with a link to the email form.
5. On the founder page remove `you@example.com` from the account: within about 3 seconds the same browser gets the app's "No access" page (403).

`GET /login?app=<slug>&sso=0` always shows the email form; that is what a failed hop falls back to, so the two hosts can never loop.
