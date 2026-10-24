// Package mail sends the emails foyrd needs: login links and invites.
package mail

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// Message is one email.
type Message struct{ To, Subject, Text, HTML string }

// Sender delivers a Message.
type Sender interface {
	Send(ctx context.Context, m Message) error
}

// Resend sends through the Resend HTTP API.
type Resend struct {
	APIKey, From string
	URL          string // defaults to Resend's endpoint
	HTTP         *http.Client
}

const resendURL = "https://api.resend.com/emails"

// Send posts the message. Errors carry the status and Resend's answer, never the API key.
func (r Resend) Send(ctx context.Context, m Message) error {
	body, err := json.Marshal(map[string]any{
		"from": r.From, "to": []string{m.To}, "subject": m.Subject, "text": m.Text, "html": m.HTML,
	})
	if err != nil {
		return err
	}
	url := r.URL
	if url == "" {
		url = resendURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return errors.New("resend: bad request")
	}
	req.Header.Set("Authorization", "Bearer "+r.APIKey)
	req.Header.Set("Content-Type", "application/json")
	client := r.HTTP
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		// The client's error wraps the URL, not our headers, but keep only the cause.
		return fmt.Errorf("resend: request failed: %w", errors.Unwrap(err))
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 200))
		return fmt.Errorf("resend: status %d: %s", resp.StatusCode, detail)
	}
	return nil
}

// LogSender writes emails to the log instead of sending them; for local development only.
type LogSender struct{ Log *slog.Logger }

// Send logs the recipient, subject and text body.
func (s LogSender) Send(ctx context.Context, m Message) error {
	log := s.Log
	if log == nil {
		log = slog.Default()
	}
	log.InfoContext(ctx, "email (not sent, log sender)", "to", m.To, "subject", m.Subject, "text", m.Text)
	return nil
}
