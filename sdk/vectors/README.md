# Assertion test vectors

`assertion.json` is the language-neutral conformance suite for Foyr identity assertions (ES256 JWS). Every SDK must pass it. The Go verifier in `internal/authn` is the reference implementation.

## Shape

```
{
  "issuer": "https://auth.foyr.online",
  "audience": "crm",
  "now": 1700000030,
  "jwks": { "keys": [ ... ] },
  "valid":   [ { "name", "token", "claims": { iss, aud, sub, email, role, iat, exp } } ],
  "invalid": [ { "name", "token", "reason" } ]
}
```

- `now` is the verifier clock in Unix seconds. Use it, not the real clock.
- Verify every token with the keys in `jwks`, the given `issuer` and `audience`, 30 s leeway and a 300 s maximum lifetime.
- Every `valid` token must verify and yield exactly the listed `claims`.
- Every `invalid` token must be rejected. `reason` is the Go error name; map it to your SDK's equivalent error kind: `ErrAssertionMalformed`, `ErrAssertionAlg`, `ErrAssertionKey`, `ErrAssertionSignature`, `ErrAssertionIssuer`, `ErrAssertionAudience`, `ErrAssertionExpired`, `ErrAssertionNotYetValid`, `ErrAssertionClaims`.

## Strictness the vectors pin

- Unknown payload claims are ignored. Unknown header members are rejected.
- Member names match exactly (case-sensitive). Duplicate names and trailing data after the JSON object are rejected, in header and payload.
- Segments are unpadded base64url only. Any other byte, including a newline, is malformed.
- The signature is exactly 64 bytes (`r||s`). High-S signatures are accepted; do not enforce low-S.
- `iat` must be 0 or more and `exp` below 2^53.

## Regenerating

Only this command rewrites the file; never edit it by hand:

```
FOYR_UPDATE_VECTORS=1 go test ./internal/authn -run TestGenerateVectors
```

ECDSA signatures are randomized, so a regenerated file differs in bytes. `TestVectorsFile` replays every vector through the Go verifier instead of diffing.
