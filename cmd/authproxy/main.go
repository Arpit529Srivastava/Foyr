// Command authproxy is Foyr's front door: it routes <slug>.<APPS_DOMAIN> to each app's live deployment
// (ADR-030), and in Phase 1 checks logins before forwarding.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Arpit529Srivastava/Foyr/internal/obs"
	"github.com/Arpit529Srivastava/Foyr/internal/proxy"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "authproxy:", err)
		os.Exit(1)
	}
}

func run() error {
	log := obs.NewLogger(os.Stdout)
	slog.SetDefault(log)
	cfg, err := proxy.Load(os.Getenv)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	table := &proxy.Table{}
	if cfg.StaticRoutes != nil {
		table.Set(cfg.StaticRoutes)
		log.Info("static routes", "routes", len(cfg.StaticRoutes))
	} else {
		go (&proxy.Poller{URL: cfg.RoutesURL, Token: cfg.Token, Namespace: cfg.Namespace, Port: cfg.AppPort,
			HTTP: &http.Client{Timeout: 5 * time.Second}, Table: table, Log: log}).Run(ctx)
	}

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           obs.RequestID(log, proxy.NewHandler(cfg.AppsDomain, table)),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	errc := make(chan error, 1)
	go func() {
		log.Info("authproxy listening", "addr", cfg.Addr, "apps_domain", cfg.AppsDomain)
		errc <- srv.ListenAndServe()
	}()
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	log.Info("authproxy shutting down")
	shutdown, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdown); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
