package github

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"
)

var (
	defaultClient = &http.Client{Timeout: 15 * time.Second}
	repoName      = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
	commitSHA     = regexp.MustCompile(`^[0-9a-f]{40}$`)
)

// App signs requests as the Foyr GitHub App.
type App struct {
	ID      int64
	Key     *rsa.PrivateKey
	BaseURL string
	HTTP    *http.Client
	Now     func() time.Time
}

// Token is a one-hour installation access token. It never prints its value.
type Token struct {
	Value     string
	ExpiresAt time.Time
}

const redacted = "ghs_[redacted]"

func (Token) String() string       { return redacted }
func (Token) GoString() string     { return redacted }
func (Token) LogValue() slog.Value { return slog.StringValue(redacted) }

// JWT returns a short-lived token proving the caller is the App (RS256, 9 minutes).
func (a *App) JWT() (string, error) {
	if a.Key == nil || a.ID <= 0 {
		return "", errors.New("github: app id and key are required")
	}
	now := a.now()
	claims, err := json.Marshal(struct {
		IssuedAt  int64 `json:"iat"`
		ExpiresAt int64 `json:"exp"`
		Issuer    int64 `json:"iss"`
	}{now.Add(-60 * time.Second).Unix(), now.Add(9 * time.Minute).Unix(), a.ID})
	if err != nil {
		return "", err
	}
	input := b64(`{"alg":"RS256","typ":"JWT"}`) + "." + b64(string(claims))
	sum := sha256.Sum256([]byte(input))
	sig, err := rsa.SignPKCS1v15(rand.Reader, a.Key, crypto.SHA256, sum[:])
	if err != nil {
		return "", fmt.Errorf("github: sign jwt: %w", err)
	}
	return input + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

// InstallationToken exchanges the App JWT for a read-only token for one installation.
func (a *App) InstallationToken(ctx context.Context, installationID int64) (Token, error) {
	if installationID <= 0 {
		return Token{}, fmt.Errorf("github: invalid installation id %d", installationID)
	}
	jwt, err := a.JWT()
	if err != nil {
		return Token{}, err
	}
	url := fmt.Sprintf("%s/app/installations/%d/access_tokens", a.BaseURL, installationID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(`{"permissions":{"contents":"read"}}`))
	if err != nil {
		return Token{}, err
	}
	setHeaders(req, "Bearer "+jwt)
	req.Header.Set("Content-Type", "application/json")

	var out struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	if err := a.do(req, http.StatusCreated, &out); err != nil {
		return Token{}, fmt.Errorf("github: installation token for %d: %w", installationID, err)
	}
	if out.Token == "" {
		return Token{}, fmt.Errorf("github: installation token for %d: empty token", installationID)
	}
	return Token{Value: out.Token, ExpiresAt: out.ExpiresAt}, nil
}

// CheckCommit confirms the token can read commit sha in repo (owner/name).
func (a *App) CheckCommit(ctx context.Context, tok Token, repo, sha string) error {
	if !repoName.MatchString(repo) || !commitSHA.MatchString(sha) {
		return fmt.Errorf("github: invalid repo %q or commit %q", repo, sha)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/repos/%s/commits/%s", a.BaseURL, repo, sha), nil)
	if err != nil {
		return err
	}
	setHeaders(req, "Bearer "+tok.Value)
	if err := a.do(req, http.StatusOK, nil); err != nil {
		return fmt.Errorf("github: read %s@%s: %w", repo, sha[:7], err)
	}
	return nil
}

// do sends req and decodes a JSON body into out when the status matches want.
func (a *App) do(req *http.Request, want int, out any) error {
	resp, err := a.client().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body := io.LimitReader(resp.Body, 1<<20)
	if resp.StatusCode != want {
		_, _ = io.Copy(io.Discard, body)
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	if out == nil {
		_, _ = io.Copy(io.Discard, body)
		return nil
	}
	return json.NewDecoder(body).Decode(out)
}

func setHeaders(req *http.Request, auth string) {
	req.Header.Set("Authorization", auth)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "foyrd")
}

func (a *App) now() time.Time {
	if a.Now != nil {
		return a.Now()
	}
	return time.Now()
}

func (a *App) client() *http.Client {
	if a.HTTP != nil {
		return a.HTTP
	}
	return defaultClient
}

func b64(s string) string { return base64.RawURLEncoding.EncodeToString([]byte(s)) }
