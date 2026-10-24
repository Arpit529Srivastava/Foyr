// Command foyrd is Foyr's control plane: API, webhooks and (later) builds and deploys.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	"github.com/Arpit529Srivastava/Foyr/internal/awsx"
	"github.com/Arpit529Srivastava/Foyr/internal/build"
	"github.com/Arpit529Srivastava/Foyr/internal/config"
	"github.com/Arpit529Srivastava/Foyr/internal/db"
	"github.com/Arpit529Srivastava/Foyr/internal/github"
	"github.com/Arpit529Srivastava/Foyr/internal/obs"
	apprun "github.com/Arpit529Srivastava/Foyr/internal/run"
	"github.com/Arpit529Srivastava/Foyr/internal/store"
	"github.com/Arpit529Srivastava/Foyr/internal/worker"
)

// maintenanceEvery is how often partitions and retention cleanup run.
const maintenanceEvery = 24 * time.Hour

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "foyrd:", err)
		os.Exit(1)
	}
}

func run() error {
	log := obs.NewLogger(os.Stdout)
	slog.SetDefault(log)

	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	applied, err := db.Migrate(ctx, pool)
	if err != nil {
		return err
	}
	log.Info("database ready", "migrations_applied", applied)
	st := store.New(pool)
	go maintain(ctx, st, log, maintenanceEvery)

	var workers sync.WaitGroup
	if cfg.Workers > 0 {
		p, err := newPool(ctx, cfg, st, log)
		if err != nil {
			return err
		}
		workers.Add(1)
		go func() { defer workers.Done(); p.Run(ctx) }()
		log.Info("build workers started", "workers", cfg.Workers, "codebuild_project", cfg.CodeBuildProject)
	}
	// Stop workers and wait for them to hand back jobs before the database closes.
	defer func() { stop(); workers.Wait() }()

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           newServer(cfg, log, st),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	internal := &http.Server{
		Addr:              cfg.InternalAddr,
		Handler:           newInternalServer(st, cfg.InternalToken, log, cfg.SessionKey),
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      10 * time.Second,
	}

	errc := make(chan error, 2)
	go func() {
		log.Info("foyrd listening", "addr", cfg.Addr, "github_app_id", cfg.GitHubAppID)
		errc <- srv.ListenAndServe()
	}()
	go func() {
		log.Info("internal listener", "addr", cfg.InternalAddr)
		errc <- internal.ListenAndServe()
	}()

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	log.Info("foyrd shutting down")
	shutdown, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_ = internal.Shutdown(shutdown)
	if err := srv.Shutdown(shutdown); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// newPool connects to AWS, checks the credentials work and returns the build worker pool.
func newPool(ctx context.Context, cfg *config.Config, st *store.Store, log *slog.Logger) (*worker.Pool, error) {
	ac, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(cfg.AWSRegion))
	if err != nil {
		return nil, fmt.Errorf("aws config: %w", err)
	}
	id, err := sts.NewFromConfig(ac).GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return nil, fmt.Errorf("aws credentials do not work (locally: aws sso login --profile foyr; or set FOYR_WORKERS=0): %w", err)
	}
	log.Info("aws ready", "account", *id.Account, "region", cfg.AWSRegion)

	logs := awsx.NewLogs(ac)
	rt := awsx.NewRuntime(ac)
	rt.Cluster, rt.Subnets, rt.SecurityGroup = cfg.ECSCluster, cfg.TenantSubnets, cfg.TenantSecurityGroup
	rt.ExecRoleARN, rt.LogGroup, rt.NamespaceID, rt.Capacity = cfg.TenantExecRoleARN, cfg.AppsLogGroup, cfg.AppsNamespaceID, cfg.AppsCapacity

	host, _ := os.Hostname()
	return &worker.Pool{
		Queue: st,
		Runners: map[string]worker.Runner{
			store.JobBuild: &build.Runner{
				Store:    st,
				Builds:   awsx.NewBuilds(ac, cfg.CodeBuildProject),
				Registry: awsx.NewRegistry(ac),
				Secrets:  awsx.NewSecrets(ac),
				Logs:     logs,
				Tokens:   &github.App{ID: cfg.GitHubAppID, Key: cfg.GitHubPrivateKey, BaseURL: cfg.GitHubAPIURL},
			},
			store.JobDeploy: &apprun.Runner{Store: st, Platform: rt, Logs: logs, LogGroup: cfg.AppsLogGroup, Region: cfg.AWSRegion},
			store.JobRetire: &apprun.Retirer{Store: st, Platform: rt},
		},
		Log:     log,
		Name:    fmt.Sprintf("%s-%d", host, os.Getpid()),
		Workers: cfg.Workers,
	}, nil
}
