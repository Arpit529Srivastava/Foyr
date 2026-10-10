// Package obs provides JSON logging and request IDs (ADR-019).
package obs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"time"
)

type (
	loggerKey    struct{}
	requestIDKey struct{}
)

// validID limits incoming request IDs to short, log-safe values.
var validID = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

// NewLogger returns a logger that writes one JSON object per line.
func NewLogger(w io.Writer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(w, nil))
}

// WithLogger stores l in ctx.
func WithLogger(ctx context.Context, l *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey{}, l)
}

// Logger returns the request-scoped logger from ctx, or slog.Default outside a request.
func Logger(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(loggerKey{}).(*slog.Logger); ok {
		return l
	}
	return slog.Default()
}

// RequestIDFrom returns the request ID set by RequestID, used as the trace ID (ADR-026).
func RequestIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

// RequestID tags every request with an ID, logs it, and logs one summary line per request.
func RequestID(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-Id")
		if !validID.MatchString(id) {
			id = newID()
		}
		w.Header().Set("X-Request-Id", id)
		l := log.With("request_id", id)
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		start := time.Now()
		ctx := context.WithValue(WithLogger(r.Context(), l), requestIDKey{}, id)
		next.ServeHTTP(rec, r.WithContext(ctx))
		// Health checks run every few seconds, so they log at debug level only.
		level := slog.LevelInfo
		if r.URL.Path == "/healthz" {
			level = slog.LevelDebug
		}
		l.Log(r.Context(), level, "request",
			"method", r.Method, "path", r.URL.Path, "status", rec.status, "duration_ms", time.Since(start).Milliseconds())
	})
}

func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// statusRecorder remembers the status code written by the handler.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }
