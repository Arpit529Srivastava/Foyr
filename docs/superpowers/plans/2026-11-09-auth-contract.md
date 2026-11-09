# Identity contract Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Every app receives a signed, verifiable identity (`X-Foyr-Assertion`), opaque `usr_` user ids, a published key set, and the `FOYR_APP_SLUG` / `FOYR_AUTH_URL` environment, exactly as the spec fixes them.

**Architecture:** Pure signing and verifying code lives in `internal/authn` (ES256 JWS, JWK set, key parsing). authproxy signs one assertion per identified request, before proxying, with its own key. foyrd publishes the public keys from config and puts the user's public id in the session token. A new migration adds the opaque id. Shared test vectors prove the signer and verifier agree for the SDKs to come.

**Tech Stack:** Go 1.27 standard library only (`crypto/ecdsa`, `crypto/x509`, `encoding/json`), Postgres 17 migrations, OpenTofu, bash.

**Spec:** `docs/superpowers/specs/2026-11-09-auth-contract-design.md`. Read it first; this plan only adds the decisions the spec leaves to an implementer.

## Global Constraints

- Go standard library only. No new module dependencies.
- Algorithm exactly `ES256`, P-256, signature as the 64-byte `r||s` of RFC 7515. Lifetime 60 s, maximum accepted lifetime 300 s, leeway 30 s.
- Public user id format `usr_` plus 16 lowercase hex characters (`^usr_[0-9a-f]{16}$`).
- Headers: `X-Foyr-User-Id`, `X-Foyr-Email`, `X-Foyr-Role`, `X-Foyr-Assertion`. Claims: `iss`, `aud`, `sub`, `email`, `role`, `iat`, `exp`. Roles: `owner`, `admin`, `member`, `viewer`.
- Variables: `FOYR_ASSERTION_KEY` (authproxy, base64 PKCS#8 P-256), `FOYR_ASSERTION_PUBLIC_KEYS` (foyrd, comma-separated base64 SPKI P-256), `FOYR_APP_SLUG`, `FOYR_AUTH_URL` (apps). SSM: `/foyr/platform/assertion-key`, `/foyr/platform/assertion-public-keys`.
- Key set: `GET /.well-known/jwks.json` on the auth host only, `Content-Type: application/json`, `Cache-Control: public, max-age=300`, `Access-Control-Allow-Origin: *`, keys sorted by `kid`.
- Never hardcode a domain; use `FOYR_AUTH_URL` and `FOYR_APPS_DOMAIN`. Config errors name the variable and never print its value.
- New tables or columns go in a new numbered migration (`007`); never edit a merged one.
- Do not touch `site/`. No em dashes in any text. Comments stay one short line.
- Work on branch `feat/auth-contract`, never on `main`. Commits are signed (`-s`), personal identity, ending with `Co-Authored-By: Claude Sonnet 5.5 <noreply@anthropic.com>`. Nothing runs against AWS in this plan; the rollout section is separate and needs the founder's approval at each step.
- Every task ends green on `gofmt -l cmd internal` (no output), `go vet ./cmd/... ./internal/...`, and `go test -race ./cmd/... ./internal/...` with `FOYR_TEST_DATABASE_URL` set (docs/dev/database.md).

## Review Focus

Inputs the spec implies but a test per task would not otherwise cover, most likely first. Each has a named test in the task that owns the code.

1. **An email with quotes, `+`, apostrophe or non-ASCII** must survive JSON encoding and still verify. Task 1, `TestAssertionRoundTripOddEmails`.
2. **Clock edges.** A verifier 30 s ahead or behind still accepts; 31 s does not. Task 1, `TestVerifyLeewayBoundaries`.
3. **Rotation overlap.** A token from the old key verifies while both keys are in the set, and fails once the old key is removed. Task 1, `TestVerifyAcrossRotation`.
4. **A session cookie issued before the deploy** (no `pid`) must send the visitor to sign in, not loop or error, and the new login must then work. Task 6, `TestSessionWithoutPublicIDIsSignedOut`.
5. **Largest email (254 characters)** must keep the assertion well under a typical 8 KB header limit. Task 1, `TestAssertionStaysSmall` (under 1,024 bytes).

## File map

| File | Responsibility |
|---|---|
| `internal/authn/assertion_keys.go` (new) | Key generation and parsing, `KeyID`, `JWKS`. No signing. |
| `internal/authn/assertion.go` (new) | Claims, `SignAssertion`, `VerifyAssertion`, error values. |
| `internal/authn/assertion_vectors_test.go` (new) | Generates and replays `sdk/vectors/assertion.json`. |
| `sdk/vectors/assertion.json`, `README.md` (new) | Language-neutral vectors for every SDK. |
| `internal/db/migrations/007_user_public_id.sql` (new) | The `users.public_id` column. |
| `internal/authn/authn.go`, `internal/store/auth.go`, `cmd/foyrd/internal.go` | `pid` in the session. |
| `internal/config/config.go`, `cmd/foyrd/jwks.go` (new), `cmd/foyrd/server.go` | foyrd key set and its config. |
| `internal/proxy/config.go`, `auth.go`, `handler.go`, `cmd/authproxy/main.go` | authproxy signing. |
| `internal/run/run.go`, `cmd/foyrd/main.go` | App environment. |
| `cmd/assertionkey/` (new), `scripts/`, `infra/` | Key tooling and wiring. |
| `docs/` | ADR-038, system design, routing, platform runbook, roadmap. |

## Deviations from the spec (found while planning)

- The spec says the vectors test "regenerates the file and fails if it differs". ECDSA signatures are randomized, so regenerated bytes never match. The test instead replays every vector through the Go verifier and fails on any mismatch. The key set in the file is deterministic and is compared exactly.
- The spec puts signing in authproxy's `rewrite`. `httputil.ReverseProxy`'s `Rewrite` cannot fail a request, so signing happens in `ServeHTTP` right after authorization, the 500 is answered there, and `rewrite` only copies the finished header. Same behavior, fail-closed.

---

### Task 1: Assertion primitives

**Files:**
- Create: `internal/authn/assertion_keys.go`, `internal/authn/assertion.go`
- Test: `internal/authn/assertion_keys_test.go`, `internal/authn/assertion_test.go`

**Interfaces:**
- Produces (later tasks rely on these exact names):
  - `func NewAssertionKeyPair() (key, public string)`: base64 PKCS#8 DER and base64 SPKI DER of a new P-256 pair.
  - `func ParseAssertionKey(b64 string) (*ecdsa.PrivateKey, error)`; `func ParseAssertionPublicKeys(csv string) ([]*ecdsa.PublicKey, error)` (at least one, duplicates dropped, P-256 only, errors never contain the input).
  - `func KeyID(pub *ecdsa.PublicKey) string` (RFC 7638 thumbprint, base64url, no padding); `func JWKS(keys []*ecdsa.PublicKey) []byte` (`{"keys":[...]}` sorted by `kid`, each `kty EC, crv P-256, alg ES256, use sig, kid, x, y`).
  - `type AssertionClaims struct{ Iss, Aud, Sub, Email, Role string; Iat, Exp int64 }` with json tags `iss aud sub email role iat exp`.
  - `func NewAssertion(iss, aud, sub, email, role string, now time.Time) AssertionClaims` (`Iat = now`, `Exp = Iat + 60`).
  - `func SignAssertion(key *ecdsa.PrivateKey, c AssertionClaims) (string, error)` (nil key is an error).
  - `type AssertionKeys map[string]*ecdsa.PublicKey`; `func KeySet(keys []*ecdsa.PublicKey) AssertionKeys` (by `KeyID`).
  - `func VerifyAssertion(keys AssertionKeys, token, issuer, audience string, now time.Time) (AssertionClaims, error)`.
  - Errors: `ErrAssertionMalformed`, `ErrAssertionAlg`, `ErrAssertionKey`, `ErrAssertionSignature`, `ErrAssertionIssuer`, `ErrAssertionAudience`, `ErrAssertionExpired`, `ErrAssertionNotYetValid`, `ErrAssertionClaims` (role not one of the four, or `exp - iat > 300`, or `exp <= iat`).
  - Unexported `verifyES256(pub *ecdsa.PublicKey, signingInput string, sig []byte) bool`, used by the RFC test.

- [ ] **Step 1: Write the failing tests** in `assertion_keys_test.go` and `assertion_test.go`:
  - `TestKeyPairRoundTrip`: `NewAssertionKeyPair` output parses with both parse functions and the public half matches the private key.
  - `TestParseRejects`: table of bad inputs (empty, not base64, Ed25519 key, P-384 key, a private key given where public is expected, trailing comma) each returns an error that does not contain the input string; duplicates in the CSV collapse to one.
  - `TestKeyIDIsAThumbprint`: `KeyID` equals the base64url SHA-256 of `{"crv":"P-256","kty":"EC","x":"<x>","y":"<y>"}` built by hand in the test; stable across calls; differs between keys.
  - `TestJWKSShapeAndOrder`: three keys in any order produce one `kid`-sorted array; every member has exactly the seven fields above; `x` and `y` decode to 32 bytes.
  - `TestVerifyES256AgainstRFC7515`: with the P-256 key and the ES256 example JWS from RFC 7515 Appendix A.3 (copy the `x`, `y`, signing input and signature from the RFC text), `verifyES256` returns true, and false after flipping one signature bit.
  - `TestAssertionRoundTripOddEmails` (Review Focus 1): emails `o'brien+x@example.com`, `"quoted"@example.com`, `ana@例え.jp` sign and verify with `Email` unchanged.
  - `TestSignatureIsAlways64Bytes`: 2,000 signatures; each decoded signature segment is exactly 64 bytes.
  - `TestVerifyRejects`: table, one row per error value: tampered payload (`ErrAssertionSignature`), `alg` `none` and `HS256` signed with the public key as secret and `crit` present (`ErrAssertionAlg`), unknown `kid` (`ErrAssertionKey`), two or four segments, padding, bad base64, signature length 63 or 65 (`ErrAssertionMalformed`), wrong `iss` (`ErrAssertionIssuer`), wrong `aud` (`ErrAssertionAudience`), `now` past `exp + 30 s` (`ErrAssertionExpired`), `now` before `iat - 30 s` (`ErrAssertionNotYetValid`), role `root`, lifetime 301 s, `exp == iat` (`ErrAssertionClaims`). Use `errors.Is`.
  - `TestVerifyLeewayBoundaries` (Review Focus 2): accepted at `exp + 29 s` and `iat - 29 s`, rejected at `exp + 31 s` and `iat - 31 s`.
  - `TestVerifyAcrossRotation` (Review Focus 3): with keys A and B in the set, tokens from each verify; with only B, the token from A returns `ErrAssertionKey`.
  - `TestAssertionStaysSmall` (Review Focus 5): a 254-character email and a 63-character slug give a token under 1,024 bytes.
  - `BenchmarkSignAssertion`.

- [ ] **Step 2: Run to verify they fail.** Run: `go test ./internal/authn -run 'Assertion|KeyPair|KeyID|JWKS|ES256|Rotation|Leeway|Signature' -v`. Expected: build failure, undefined symbols.

- [ ] **Step 3: Implement the interfaces above.** Approach: use `ecdsa.Sign` (returns `r` and `s`), left-pad `r` and `s` to 32 bytes; verify with `ecdsa.Verify` on the SHA-256 of the signing input; decode the header strictly (`json.Decoder` with `DisallowUnknownFields` on a header struct that includes `crit` so its presence is caught); `x509.MarshalPKCS8PrivateKey` / `MarshalPKIXPublicKey` for key encoding.

- [ ] **Step 4: Run to verify they pass.** Same command. Expected: PASS. Then `go test -bench SignAssertion -run '^$' ./internal/authn` and note ns/op (budget 200,000 ns/op); record it for the PR.

- [ ] **Step 5: Commit.** `git add internal/authn && git commit -s -m "feat(authn): ES256 identity assertion, key set and key parsing"`.

### Task 2: Shared test vectors

**Files:**
- Create: `internal/authn/assertion_vectors_test.go`, `sdk/vectors/assertion.json`, `sdk/vectors/README.md`

**Interfaces:**
- Consumes: everything from Task 1.
- Produces: `sdk/vectors/assertion.json` with this shape: `{"issuer","audience","now"(unix s),"jwks":{...},"valid":[{"name","token","claims":{...}}],"invalid":[{"name","token","reason"}]}`, where `reason` is the Go error value's name (for example `ErrAssertionExpired`). Fixed audience `crm`, issuer `https://auth.foyr.online`, `now` 1700000030, tokens issued at 1700000000.

- [ ] **Step 1: Write the failing test** `TestVectorsFile`: reads `../../sdk/vectors/assertion.json`; asserts the file's `jwks` equals `JWKS` of the fixed test key byte for byte; every `valid` token verifies and its claims equal the listed ones; every `invalid` token fails with exactly the named error; the set of `reason` values covers every `ErrAssertion*` value and includes the named cases `alg-none`, `alg-hs256-public-key-as-secret`, `unknown-kid`, `truncated-signature`, `signature-65-bytes`, `lifetime-301s`, `unknown-role`, `tampered-payload`, `wrong-audience`, `wrong-issuer`, `expired`, `not-yet-valid` (as `name`). Also `TestGenerateVectors`, skipped unless `FOYR_UPDATE_VECTORS=1`, which rewrites the file from a fixed test private key declared as a test-only constant (comment: never used outside tests).

- [ ] **Step 2: Run to verify it fails.** `go test ./internal/authn -run Vectors -v`. Expected: FAIL, file missing.

- [ ] **Step 3: Generate and write the README.** Run `FOYR_UPDATE_VECTORS=1 go test ./internal/authn -run TestGenerateVectors`. The README explains the shape, that Go is the reference, and that the file is rewritten only by that command.

- [ ] **Step 4: Run to verify it passes.** `go test ./internal/authn -run Vectors -v`. Expected: PASS.

- [ ] **Step 5: Commit.** `git add internal/authn sdk/vectors && git commit -s -m "test(authn): shared assertion vectors for the SDKs"`.

### Task 3: Opaque user ids (migration 007)

**Files:**
- Create: `internal/db/migrations/007_user_public_id.sql`
- Modify: `internal/db/db_test.go`

**Interfaces:**
- Produces: `users.public_id text NOT NULL UNIQUE`, default `'usr_' || substr(replace(gen_random_uuid()::text, '-', ''), 1, 16)`, check `public_id ~ '^usr_[0-9a-f]{16}$'`. Inserts that name only `email` keep working.

- [ ] **Step 1: Write the failing test** `TestMigratePublicIDs` in `db_test.go`, following `TestMigrateAuthTables`: apply `001_init` to `006_dashboard_login` by hand, insert three users (`email` only), call `db.Migrate`, then assert: every existing user has a `public_id` matching `^usr_[0-9a-f]{16}$`; the three are distinct; a new `INSERT INTO users (email)` gets one; `UPDATE users SET public_id = 'usr_XYZ'` fails (check); setting two users to the same valid id fails (unique); `public_id` cannot be set to NULL.

- [ ] **Step 2: Run to verify it fails.** `go test ./internal/db -run TestMigratePublicIDs -v`. Expected: FAIL (column missing).

- [ ] **Step 3: Write the migration.** Add the column with the volatile default (Postgres fills each existing row), then `NOT NULL`, the unique constraint `users_public_id_key`, and the check `users_public_id_format`. Header comment: additive, a rolled-back foyrd still inserts correctly because of the default.

- [ ] **Step 4: Run to verify it passes**, and run the whole `./internal/db` package: `TestMigrateAppliesOnce` compares against the directory listing, so it needs no edit. Expected: PASS.

- [ ] **Step 5: Commit.** `git add internal/db && git commit -s -m "feat(db): opaque public id for users (migration 007)"`.

### Task 4: `pid` in the session

**Files:**
- Modify: `internal/authn/authn.go` (`Claims`), `internal/store/auth.go` (`Session`, `ExchangeCode`), `cmd/foyrd/internal.go` (`exchange`)
- Test: `internal/store/auth_test.go`, `cmd/foyrd/internal_test.go`

**Interfaces:**
- Consumes: migration 007.
- Produces: `authn.Claims.PublicID string` (json `pid,omitempty`); `store.Session.PublicID string`, filled by `ExchangeCode` from `users.public_id` (add it to the `RETURNING` list).

- [ ] **Step 1: Write the failing tests.** `TestExchangeCodeReturnsThePublicID` in the store tests: after a login the returned `Session.PublicID` matches `^usr_[0-9a-f]{16}$` and equals the value read directly from `users`. Extend `TestInternalExchangeIssuesAVerifiableToken`: `claims.PublicID` is that same value, and a second user gets a different one.

- [ ] **Step 2: Run to verify they fail.** `go test ./internal/store ./cmd/foyrd -run 'PublicID|ExchangeIssues' -v`. Expected: FAIL, field missing.

- [ ] **Step 3: Implement.** Add the field and tag, return the column from the existing `UPDATE ... RETURNING`, and pass `PublicID: sess.PublicID` in `exchange`. Dashboard sessions are separate and unchanged.

- [ ] **Step 4: Run to verify they pass.** Same command. Expected: PASS.

- [ ] **Step 5: Commit.** `git add internal cmd && git commit -s -m "feat(auth): carry the user's public id in the session"`.

### Task 5: foyrd publishes the key set

**Files:**
- Create: `cmd/foyrd/jwks.go`
- Modify: `internal/config/config.go` (`Config`, `loadLogin`), `cmd/foyrd/server.go`
- Test: `internal/config/config_test.go`, `cmd/foyrd/server_test.go`

**Interfaces:**
- Consumes: `ParseAssertionPublicKeys`, `JWKS`.
- Produces: `Config.AssertionPublicKeys []*ecdsa.PublicKey`; `func jwksHandler(keys []*ecdsa.PublicKey) http.Handler` in `cmd/foyrd/jwks.go` (body computed once; sets the three headers from Global Constraints; `GET` only).

- [ ] **Step 1: Write the failing tests.**
  - Config: extend `loginEnv` with a valid `FOYR_ASSERTION_PUBLIC_KEYS`. `TestLoginSettings` asserts `len(c.AssertionPublicKeys) == 1`. Add rows to `TestLoginSettingsRejected`: missing (error mentions `FOYR_ASSERTION_PUBLIC_KEYS is not set`), malformed, a P-384 key, a private key, and each error must not contain the value. Login off (`FOYR_SESSION_KEY` unset) needs no key.
  - Server, following `TestLoginRoutesExistOnlyWhenLoginIsOn`: `TestKeySetServedOnTheAuthHostOnly` with login on and `AuthURL` `https://auth.test`: request with `Host: auth.test` returns 200, body equals `authn.JWKS(keys)`, and the three headers; the same path on `Host: app.test` and `Host: api.test` returns 404; `POST` returns 405; with login off every host returns 404. `TestKeySetListsEveryConfiguredKey` with two keys in reverse order: body lists both, sorted.

- [ ] **Step 2: Run to verify they fail.** `go test ./internal/config ./cmd/foyrd -run 'Login|KeySet' -v`. Expected: FAIL.

- [ ] **Step 3: Implement.** Parse in `loadLogin` after the session key, required whenever login is on, error text names the variable only. In `server.go`, register `GET /.well-known/jwks.json` when `cfg.LoginOn()`, wrapped so that any host other than the host of `cfg.AuthURL` gets `http.NotFound`.

- [ ] **Step 4: Run to verify they pass.** Same command. Expected: PASS.

- [ ] **Step 5: Commit.** `git add internal cmd && git commit -s -m "feat(foyrd): publish the identity key set on the auth host"`.

### Task 6: authproxy signs the assertion

**Files:**
- Modify: `internal/proxy/config.go`, `internal/proxy/auth.go`, `internal/proxy/handler.go`, `cmd/authproxy/main.go`
- Test: `internal/proxy/config_test.go`, `internal/proxy/auth_test.go`

**Interfaces:**
- Consumes: `ParseAssertionKey`, `NewAssertion`, `SignAssertion`, `Claims.PublicID`.
- Produces: `proxy.Config.AssertionKey *ecdsa.PrivateKey`; `proxy.Auth.AssertionKey *ecdsa.PrivateKey`; `identity.publicID string`; `func (a *Auth) assertion(slug string, id *identity) (string, error)` (nil key is an error); `route.assertion string`.

- [ ] **Step 1: Write the failing tests.**
  - Config: `TestLoadAuthSettings` gains `FOYR_ASSERTION_KEY`; rows for missing (`FOYR_ASSERTION_KEY is not set`), malformed, P-384 and a public key; none echo the value; auth off needs nothing.
  - Rig: `newAuthRig` generates an assertion key pair, sets `Auth.AssertionKey`, keeps the public half; `session()` takes a public id (default `usr_0123456789abcdef`) and sets `Claims.PublicID`. Update the existing identity assertions from `"7"` and `"42"` to that id.
  - `TestAssertionAccompaniesIdentity`: a member of `crm` (role `admin`) gets all four headers; `VerifyAssertion(KeySet([]*ecdsa.PublicKey{pub}), tok, testAuthURL, "crm", now)` succeeds with `Sub == usr_...`, `Email == "ana@example.com"`, `Role == "admin"`, `Exp - Iat == 60`; the token's header `kid` equals `KeyID(pub)`.
  - `TestViewerOfPublicAppGetsAViewerAssertion`: `blog`, role `viewer`.
  - `TestAnonymousVisitorGetsNoIdentityHeaders`: none of the four headers.
  - `TestVisitorSentAssertionIsDeleted`: a visitor-sent `X-Foyr-Assertion` never reaches the app, anonymous or signed in; the signed-in app sees only the real one.
  - `TestSessionWithoutPublicIDIsSignedOut` (Review Focus 4): a cookie whose claims lack `PublicID` on private `crm` gets the normal login redirect for browsers and the 401 JSON for API calls; the app is not hit; then the same visitor with a fresh session carrying `PublicID` is let in.
  - `TestSigningFailureAnswers500AndForwardsNothing`: `Auth.AssertionKey = nil` with a valid session gives 500 and `g.hits == 0`.

- [ ] **Step 2: Run to verify they fail.** `go test ./internal/proxy -v`. Expected: FAIL.

- [ ] **Step 3: Implement.** `loadAuth` parses the key, required once `FOYR_SESSION_PUBLIC_KEY` is set. `session()` treats empty `claims.PublicID` as no session. In `ServeHTTP`, after `authorize` returns an identity, build and sign the assertion with `a.now()`; on error log (no key material) and answer 500 with the existing error page helper; store the token in `route.assertion`. `rewrite` sets `X-Foyr-User-Id` to `id.publicID` and `X-Foyr-Assertion` to `rt.assertion` when identified. Wire `AssertionKey` in `cmd/authproxy/main.go`.

- [ ] **Step 4: Run to verify they pass.** `go test -race ./internal/proxy ./cmd/authproxy`. Expected: PASS.

- [ ] **Step 5: Commit.** `git add internal cmd && git commit -s -m "feat(authproxy): sign an identity assertion per request, send usr_ ids"`.

### Task 7: App environment

**Files:**
- Modify: `internal/run/run.go` (`Runner`, deploy), `cmd/foyrd/main.go`
- Test: `internal/run/run_test.go`

**Interfaces:**
- Produces: `Runner.AuthURL string`; `func appEnv(slug, authURL string) map[string]string` returning `PORT`, `NODE_ENV=production`, `FOYR_APP_SLUG`, and `FOYR_AUTH_URL` only when `authURL != ""`.

- [ ] **Step 1: Write the failing tests.** `TestAppEnv`: table covering with and without an auth URL. Extend `TestDeployGoesLive`: with `AuthURL` set on the rig runner, `spec.Env["FOYR_APP_SLUG"]` equals the app's slug read from the store and `spec.Env["FOYR_AUTH_URL"]` equals the configured URL. `TestDeployFailsWithoutASlug`: after `UPDATE apps SET slug = NULL`, the deployment ends `failed` at step `Start` and no task definition is registered.

- [ ] **Step 2: Run to verify they fail.** `go test ./internal/run -run 'AppEnv|DeployGoesLive|WithoutASlug' -v`. Expected: FAIL.

- [ ] **Step 3: Implement.** Load the app with `r.Store.GetApp` before `RegisterTask`; an empty slug goes through the existing `r.fail(ctx, d, "Start", "This app has no address yet.", "")` path. Set `AuthURL: cfg.AuthURL` in `cmd/foyrd/main.go`.

- [ ] **Step 4: Run to verify they pass.** Same command. Expected: PASS.

- [ ] **Step 5: Commit.** `git add internal cmd && git commit -s -m "feat(run): give every app FOYR_APP_SLUG and FOYR_AUTH_URL"`.

### Task 8: Key tooling and infra wiring

**Files:**
- Create: `cmd/assertionkey/main.go`, `cmd/assertionkey/main_test.go`
- Modify: `scripts/platform-secrets.sh`, `scripts/login-dev-env.sh`, `scripts/authproxy-dev.sh`, `scripts/foyrd-dev.sh`, `infra/authproxy.tf`, `infra/foyrd.tf`, `infra/infra.tftest.hcl`

**Interfaces:**
- Produces: `func output() string` in `cmd/assertionkey` returning `key=<b64>\npublic=<b64>\n`.

- [ ] **Step 1: Write the failing tests.**
  - `TestOutputIsAUsablePair` in `cmd/assertionkey`: both lines parse with `ParseAssertionKey` and `ParseAssertionPublicKeys`; a token signed with the key verifies against the public one.
  - tftest `run "assertion_secrets"`: foyrd's secrets include `FOYR_ASSERTION_PUBLIC_KEYS` ending `parameter/foyr/platform/assertion-public-keys` and no secret ending `/assertion-key`; authproxy's include `FOYR_ASSERTION_KEY` ending `parameter/foyr/platform/assertion-key`; no assertion value is a plain environment entry.

- [ ] **Step 2: Run to verify they fail.** `go test ./cmd/assertionkey`; for infra install the OpenTofu release binary matching `infra/versions.tf` into the scratchpad (not brew), then `tofu -chdir=infra init -backend=false && tofu -chdir=infra test`. Expected: FAIL.

- [ ] **Step 3: Implement.** `main` prints `output()`. In `platform-secrets.sh`, make the pair once into `~/.config/foyr/assertion.{key,pub}` and `put` both parameters only when `aws ssm get-parameter` says they do not exist yet (rotation state in SSM must never be overwritten by a re-run); comment saying so. Dev scripts: `login-dev-env.sh` creates `dev-assertion.{key,pub}`; `authproxy-dev.sh` exports `FOYR_ASSERTION_KEY`, `foyrd-dev.sh` exports `FOYR_ASSERTION_PUBLIC_KEYS`. Add the one secret to each task definition (`foyrd_secrets` map entry for foyrd).

- [ ] **Step 4: Run to verify they pass.** `go test ./cmd/assertionkey`; `shellcheck scripts/*.sh`; `tofu fmt -check -recursive infra`; `tofu -chdir=infra validate`; `tofu -chdir=infra test`; the same three in `infra/bootstrap`. Expected: all pass, no shellcheck findings.

- [ ] **Step 5: Commit.** `git add cmd scripts infra && git commit -s -m "feat(infra): assertion key tooling and secret wiring"`.

### Task 9: Docs

**Files:**
- Modify: `docs/architecture/decisions.md`, `docs/architecture/system-design.md` (sections 4e and 7), `docs/dev/routing.md`, `docs/dev/platform.md`, `docs/roadmap.md`

**Interfaces:** none.

- [ ] **Step 1: Write ADR-038** after ADR-037, in the same Decision / Why / Consequences form, recording: ES256 and why; opaque ids; own key plus `kid` and key set; `pid` in the session; sessions without it are signed out once; env variables, with the `FOYR_` prefix reserved so the future Environment tab must refuse customer variables with it; the two deviations above; the signing benchmark result from Task 1. **Update system design 4e and 7**: the assertion is ES256, verified against the key set at `auth.<PLATFORM_DOMAIN>/.well-known/jwks.json`, ids are `usr_...`. **routing.md**: replace the three-header description with the four-header table. **roadmap.md**: the authproxy item reads "signed assertion: ADR-038"; the `@foyr/auth` SDK item stays open. **platform.md**: add "Identity keys (ADR-038)": creating the pair with `scripts/platform-secrets.sh`, and the four-step rotation runbook from the spec with the exact `aws ssm put-parameter` and deploy steps.

- [ ] **Step 2: Verify.** `grep -c $'\xe2\x80\x94' docs/architecture/decisions.md docs/dev/platform.md docs/dev/routing.md docs/roadmap.md docs/architecture/system-design.md` prints 0 for each; every variable and SSM name in the docs matches the Global Constraints list (`grep -rn 'ASSERTION' docs`).

- [ ] **Step 3: Commit.** `git add docs && git commit -s -m "docs: ADR-038 identity contract, runbook and routing"`.

### Task 10: Whole-branch verification and PR

- [ ] **Step 1: Run everything.** `gofmt -l cmd internal` (no output); `go vet ./cmd/... ./internal/...`; `go test -race -count=1 ./cmd/... ./internal/...` with a throwaway Postgres and `FOYR_TEST_DATABASE_URL` (TCP only, `unix_socket_directories=''`); the tofu and shellcheck commands from Task 8. Expected: all green.
- [ ] **Step 2: Push and open the PR** from `feat/auth-contract` with a body listing the spec link, the benchmark number, the two deviations, and the rollout section below as the merge-then-deploy checklist. Do not merge or deploy.

## Rollout (after merge; each step needs the founder's approval)

Deploys are manual while GitHub Actions is billing-locked: build the arm64 binary, push the image with `crane`, register a task definition copying the latest revision with only the image swapped, update the service. The same method as 2026-11-09.

1. `scripts/platform-secrets.sh` writes `assertion-key` and `assertion-public-keys` (create-only). Confirm with `aws ssm get-parameter --name /foyr/platform/assertion-public-keys --query Parameter.Version`.
2. `tofu -chdir=infra plan`: expect only two new task definition revisions and no service changes (`ignore_changes`). Then `apply`.
3. Deploy foyrd. Check `https://auth.foyr.online/.well-known/jwks.json` returns one key, `https://app.foyr.online/.well-known/jwks.json` returns 404, foyrd logs show migration 007 and no errors.
4. Deploy authproxy. Sign in at a private app; expect one fresh magic-link login.
5. Live check: an app shows `X-Foyr-User-Id` as `usr_...` and an `X-Foyr-Assertion` that verifies with `VerifyAssertion` against the live key set (a one-off Go test run, not shipped). `foyr-hello` needs a redeploy to show the new environment; touching its repository is the founder's call.

Rollback: authproxy to its previous task definition, then foyrd to its previous one. The column stays.
