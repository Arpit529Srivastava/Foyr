# The identity contract: what every app receives

Status: draft for the founder's review, 2026-11-09. Sub-project 1 of 2. Sub-project 2 is the `@foyr/auth` package, written after this one ships.

## Why

`@foyr/auth` is a public interface. Once customers write `getUser` and `verifyUser` into their apps and store user ids in their own databases, the shapes below cannot change without breaking them. So this spec fixes the contract first, and builds only the platform side of it. The SDK is a thin reader of this contract and gets its own spec.

Today authproxy sends three plain headers (`X-Foyr-User-Id`, `X-Foyr-Email`, `X-Foyr-Role`, `internal/proxy/handler.go:126`). The published docs and ADR-006 promise more: a signed `X-Foyr-Assertion` that an app can verify against Foyr's published keys, an opaque user id like `usr_8f2k1c`, and a `FOYR_DEV_USER` local mode that is ignored on Foyr. None of that exists yet. The user id is the database row number.

## Goals

- An app can prove, without trusting the network, who the visitor is, which app the proof is for, and that Foyr said so.
- User ids are opaque, stable, and safe to store in a customer's database for the life of the product.
- Keys can be rotated without breaking any app, with no downtime and no coordinated deploy.
- Nothing here is a stepping stone. Each piece is the final shape.

## Non-goals

- The SDK package, its npm release, and the public docs pages (sub-project 2).
- `hasPermission` and `foyr.json` (roadmap Phase 2). Roles are already carried and stay `owner`, `admin`, `member`, `viewer`.
- API tokens for machines, custom domains, scale-to-zero.
- Any change to the dashboard or founder pages.

## The contract

This is the whole public surface. Everything after this section is how it is built.

### Headers an app receives for a signed-in visitor

| Header | Value |
|---|---|
| `X-Foyr-User-Id` | `usr_` plus 16 lowercase hex characters. Opaque, unique across Foyr, never reused. |
| `X-Foyr-Email` | The visitor's email, lowercased. |
| `X-Foyr-Role` | `owner`, `admin`, `member` or `viewer`. A signed-in non-member of a public app is `viewer`. |
| `X-Foyr-Assertion` | A signed token carrying the same facts, valid for 60 seconds. |

Anonymous visitors of a public app get none of the four. Any `X-Foyr-*` header a visitor sends is deleted before the app sees the request (already true, kept).

The plain headers stay supported for good. The docs offer `getUser` (headers) and `verifyUser` (signed) as two real choices.

### The assertion

A JWS compact token, algorithm ES256 (ECDSA P-256 with SHA-256, signature as the fixed 64-byte `r||s` of RFC 7515).

Header: `{"alg":"ES256","typ":"JWT","kid":"<thumbprint>"}`

Claims:

| Claim | Meaning |
|---|---|
| `iss` | The auth URL, for example `https://auth.foyr.online`. Exactly `FOYR_AUTH_URL`. |
| `aud` | The app's slug, a single string. |
| `sub` | The public user id. |
| `email` | As in the header. |
| `role` | As in the header. |
| `iat`, `exp` | Unix seconds. `exp` is `iat + 60`. |

Rules for anything that verifies one (the Go reference verifier and every SDK): `alg` must be exactly `ES256` and there must be no `crit`; the token has three base64url segments without padding and a 64-byte signature; `kid` must name a key in the published set; `iss` and `aud` must match what the verifier expects; `role` must be one of the four; `exp - iat` must be at most 300; the token is valid while `iat <= now + 30s` and `now < exp + 30s`. Anything else is rejected.

`kid` is the RFC 7638 thumbprint of the public key, base64url. It is computed from the key and never configured, so it cannot drift.

### Published keys

`GET https://auth.<PLATFORM_DOMAIN>/.well-known/jwks.json` returns a JWK Set:

```json
{"keys":[{"kty":"EC","crv":"P-256","alg":"ES256","use":"sig","kid":"…","x":"…","y":"…"}]}
```

Served by foyrd on the auth host only (404 on every other host), `Content-Type: application/json`, `Cache-Control: public, max-age=300`, `Access-Control-Allow-Origin: *`. Keys are sorted by `kid` so the body is stable. The set always lists every key that may have signed a token that has not yet expired.

### Environment every app gets

| Variable | Value |
|---|---|
| `FOYR_APP_SLUG` | The app's slug. The only trusted source of the audience, and the signal for "I am running on Foyr". |
| `FOYR_AUTH_URL` | The auth URL. Where the issuer and the keys live, so nothing in an SDK hard-codes a domain. |

The `FOYR_` prefix is reserved: when the Environment tab ships, customer variables with that prefix must be refused. Apps already running get these on their next deploy. Until then an SDK must say so clearly rather than guess.

## Design

### 1. Opaque user ids

Migration `007_user_public_id.sql` adds `users.public_id text NOT NULL UNIQUE` with a database default of `'usr_' || substr(replace(gen_random_uuid()::text, '-', ''), 1, 16)` and a check `public_id ~ '^usr_[0-9a-f]{16}$'`. Postgres fills every existing row from the volatile default, and every future insert gets one with no application code. 64 bits of a random UUID make a collision negligible, and the unique constraint turns the impossible case into an error instead of a bug.

The migration is additive and the new column has a default, so a rolled-back foyrd still inserts users correctly.

The id travels in the session token. `authn.Claims` gains `PublicID` (`pid`), set when foyrd exchanges a login code (`cmd/foyrd/internal.go`, `exchange`), from the user row. The numeric id stays inside the platform for membership, revocation and audit, and never appears in a header, token or page an app can see.

Sessions issued before this change have no `pid`. authproxy treats a session without one as signed out, so each person signs in again once, by magic link. Only the founder's own test accounts exist today, so this costs nothing now and avoids a permanent fallback path in the proxy.

### 2. Signing, in authproxy

authproxy signs one assertion per proxied request that has an identity, in `rewrite`, after the existing step that deletes every incoming `X-Foyr-*` header. Signing is a pure function in a new file, `internal/authn/assertion.go`, next to the session token code: it takes a key, claims and a clock, and does no I/O. ES256 signing costs tens of microseconds, so there is no cache. A benchmark guards a budget of 200 microseconds, and caching per session is left out until a measurement asks for it.

If signing ever fails, the request is answered with a 500 and nothing is forwarded. The app never receives identity headers without their assertion.

authproxy gets its own key, `FOYR_ASSERTION_KEY`: base64 of a PKCS#8 P-256 private key. The session key is not reused, for two reasons. authproxy holds only the public session key on purpose, so it can check sessions but never create one, and that property must survive. And a separate key family and format mean a session token can never be mistaken for an assertion or the reverse. When login is on, a missing or invalid assertion key is a startup error, like the session key (ADR-035): the task fails and the circuit breaker rolls back, rather than quietly serving apps without proof.

### 3. Keys and rotation

foyrd gets `FOYR_ASSERTION_PUBLIC_KEYS`: a comma-separated list of base64 SPKI public keys, at least one when login is on, each checked to be P-256, duplicates dropped. It publishes them as the JWK Set. foyrd never holds a private assertion key.

Rotation is four steps, each safe on its own, with no deploy that must land at the same moment as another:

1. Add the new public key to the list, deploy foyrd. The set now lists old and new.
2. Wait at least an hour, so every SDK cache has seen the set.
3. Replace `assertion-key` with the new private key, deploy authproxy. Old and new tasks overlap during the deploy and both signing keys are in the set.
4. After at least two minutes (one assertion lifetime plus leeway), remove the old public key from the list, deploy foyrd.

The runbook goes in `docs/dev/platform.md`. SDKs refetch the set once when they meet an unknown `kid`, so a missed wait degrades to one extra request, not to an outage.

Generation: a new command `cmd/assertionkey` prints `key=` and `public=` lines, like `cmd/sessionkey`. `scripts/platform-secrets.sh` creates the pair once, keeps it in `~/.config/foyr/assertion.{key,pub}` and writes SSM `/foyr/platform/assertion-key` and `/foyr/platform/assertion-public-keys`. Local scripts (`authproxy-dev.sh`, `foyrd-dev.sh`) generate a throwaway pair.

### 4. The keys endpoint

A small handler in foyrd, built from config only, with no database access, so it stays up when the database is slow. Registered only on the host of `FOYR_AUTH_URL`, the way the customer sign-in pages are registered only on the dashboard host.

### 5. App environment

`internal/run` already builds the task definition for each deployment with `PORT` and `NODE_ENV` (`run.go:100`). It adds `FOYR_APP_SLUG` from the app row and `FOYR_AUTH_URL` from foyrd's config. An app without a slug cannot be deployed, which is already true for routing.

### 6. Failure and threat notes

- A compromised authproxy can mint assertions. It could already set any identity header, so nothing is lost, and the stronger property (it cannot create sessions) is kept.
- An assertion left in an app's logs is useless after a minute. Its contents (email, role) are already in the plain headers.
- `aud` plus the network rules (tenants cannot reach each other, authproxy deletes inbound `X-Foyr-*`) mean an assertion cannot be replayed to a different app. The audience check is for the day those assumptions loosen, such as custom domains.
- The signing key is a SecureString in SSM, injected by ECS at task start, and is never logged. Config errors name the variable, never its value.

## Where the change lands

| Area | Change |
|---|---|
| `internal/db/migrations/007_user_public_id.sql` | New. The column, default, constraints. |
| `internal/authn/authn.go` | `Claims.PublicID`. |
| `internal/authn/assertion.go` | New. Sign, verify (the reference), JWK and thumbprint, key parsing. |
| `internal/store/auth.go` | The login exchange returns the user's public id. |
| `cmd/foyrd/internal.go`, `server.go` | Put `pid` in the session. Serve the key set. |
| `internal/config/config.go` | `FOYR_ASSERTION_PUBLIC_KEYS`, validated. |
| `internal/proxy/auth.go`, `handler.go`, `config.go`, `cmd/authproxy/main.go` | Reject sessions without `pid`. Sign and send the assertion. `FOYR_ASSERTION_KEY`. |
| `internal/run/run.go`, `cmd/foyrd/main.go` | The two environment variables. |
| `cmd/assertionkey`, `scripts/platform-secrets.sh`, dev scripts | Key generation and storage. |
| `infra/authproxy.tf`, `infra/foyrd.tf`, `infra/infra.tftest.hcl` | One secret each, plus tests. |
| `sdk/vectors/assertion.json` | New. Shared test vectors, see below. |
| `docs/` | ADR-038, system design 4e and 7, `routing.md`, `platform.md`, roadmap. |

## Shared test vectors

`sdk/vectors/assertion.json` is generated by Go from a fixed test key and a fixed clock, and holds one key set, valid tokens, and a list of invalid ones with the reason each must fail: expired, not yet valid, wrong audience, wrong issuer, tampered payload, `alg: none`, `alg: HS256` signed with the public key as the secret, unknown `kid`, truncated signature, wrong signature length, too long a lifetime, unknown role. A Go test regenerates the file in memory and fails if it differs, so it cannot drift from the signer. It lives outside any one language's folder on purpose: the JavaScript SDK and a later Python SDK both read it, and it is the written proof that two languages agree.

As an independent check on the signature format, the verifier is also tested against the published ES256 example in RFC 7515 Appendix A.3, which no Foyr code produced.

## Testing

Table-driven Go tests, run with `-race`, with the database tests on Postgres.

- Assertion: round trip; the raw signature is always 64 bytes (a loop of 2,000 signatures reaches the short `r` and `s` cases); every invalid vector is rejected for its stated reason; the RFC example verifies; thumbprint matches an independent computation.
- authproxy: the four headers present for members and for viewers of public apps, absent for anonymous; a visitor-sent `X-Foyr-Assertion` is deleted; `aud`, `iss`, `sub`, `role` and `exp` are right; the `kid` is in the served key set; a session without `pid` is treated as signed out; signing failure answers 500 and forwards nothing; startup fails without a valid key.
- foyrd: the key set is served on the auth host and is 404 elsewhere, with the right headers; config rejects a missing, malformed or non-P-256 key; the exchange puts `pid` in the session; new app task definitions carry both variables.
- Store: migration 007 backfills existing rows uniquely, rejects a malformed id, and gives new users an id with no application code.
- Infra: `tofu test` checks both secrets are wired.

## Rollout and rollback

Each step is safe alone. Steps marked (approval) need the founder's go-ahead, and deploys are manual while GitHub Actions is billing-locked.

1. Merge the code. Nothing runs yet.
2. (approval) Create the keys and write SSM with `scripts/platform-secrets.sh`.
3. (approval) `tofu apply`. This only registers new task definition revisions carrying the new secrets. Both services use `ignore_changes = [task_definition]`, so nothing restarts and nothing changes yet. The plan step confirms this with `tofu plan` before applying.
4. (approval) Deploy foyrd. Migration 007 runs, sessions start carrying `pid`, the key set goes live. The still-running old authproxy ignores the new claim and keeps sending numeric ids, so this step is invisible to apps.
5. (approval) Deploy authproxy. It starts signing, and ids in headers become `usr_…`. Anyone signed in signs in once more.
6. Live check: the key set answers; an app receives the four headers; the assertion verifies against the live key set.

Rollback: authproxy back to the previous task definition (the new key is unused). foyrd back to the previous one (the column stays, harmless). Order matters only going forward: foyrd before authproxy.

## Decisions recorded

| Decision | Why | Rejected |
|---|---|---|
| Opaque ids in a new column | Customers store the id for ever. Row numbers leak the user count and could never be swapped. | Numeric ids as strings (cannot be fixed later). Prefixing the number (still sequential). |
| ES256 | The docs already say so. Every JWT library and runtime verifies it, including edge runtimes and Python. | EdDSA (not available in every runtime Foyr apps will use). |
| Own key for authproxy, `kid` plus a key set | Keeps "authproxy cannot create sessions". Rotation without coordination. | Reusing the session key. A single fixed key. |
| `pid` in the session, old sessions rejected | One path, no fallback code kept for ever. One re-login now, when only the founder has accounts. | Looking the id up per request. Accepting old sessions. |
| Keys endpoint in foyrd, config only | No database in the path. foyrd already serves the auth host. | Serving it from authproxy. |
| `FOYR_APP_SLUG` and `FOYR_AUTH_URL` injected | A trusted audience, and no domain baked into a published package. | An SDK default of `auth.foyr.online`. |
| 60 second lifetime, sign per request | No replay store, no cache to invalidate. | Longer tokens with a cache. |

## Risks

- Everyone signed in at step 5 signs in once more. Acceptable now, and the reason this ships before outside users.
- authproxy gains a signing key, which raises what a compromise of it can do (see the threat notes). Accepted, and the session key stays out of it.
- Apps that are already deployed lack the two variables until redeployed. `getUser` works for them. `verifyUser` will say what to do.
- Step 3 relies on `ignore_changes` keeping services still. The plan checks it with `tofu plan` first.
- The live check needs an app that can show what it received. `foyr-hello` can, but it also needs a redeploy to get the variables, which touches its repository and is the founder's call.

## Hand-off to sub-project 2

The SDK spec starts from this contract and the vectors: `getUser` reads the plain headers; `verifyUser` fetches and caches the key set (refetch once on an unknown `kid`, serve stale on network failure), checks the audience from `FOYR_APP_SLUG`, throws on invalid input; `FOYR_DEV_USER` is ignored when `FOYR_APP_SLUG` is set. It also corrects the public docs: ids look like `usr_` plus hex, and the example in the auth page changes. npm publishing is the founder's step.

## Amendments during implementation (2026-11-09)

Recorded in ADR-038. These refine or correct the text above; where they differ, they win.

- **Ruling A, verifier strictness.** Known member names match exactly (case-sensitive). Duplicate member names are rejected in header and payload, and so is trailing data after the JSON object. Unknown header members are rejected. Unknown payload claims are ignored, so a later claim cannot break deployed SDKs. Segments may contain only `[A-Za-z0-9_-]`.
- **Ruling B, high-S.** High-S signatures are accepted (RFC 7515 allows both). Apps must never use the token string as an identity or cache key.
- **Ruling C, time bounds.** `iat` must be 0 or more and `exp` below 2^53. `exp - iat` must be at most 300 seconds. Leeway is 30 seconds. The assertion lasts 60 seconds.
- **Deviation 1, vectors test.** It replays every token through the verifier instead of regenerating the file, because ECDSA signatures are randomized. The key set in the file is deterministic and compared exactly.
- **Deviation 2, where signing happens.** In authproxy's `ServeHTTP` right after authorization, not inside `rewrite`, because `Rewrite` cannot fail a request. A signing failure answers 500 and forwards nothing.
