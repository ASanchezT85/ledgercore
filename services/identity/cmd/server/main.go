// Command server runs the LedgerCore identity service: tenant registry,
// API key management and EdDSA JWT issuance (with JWKS publication).
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/ledgercore/ledgercore/libs/go/events"
	"github.com/ledgercore/ledgercore/libs/go/obs"
	"github.com/ledgercore/ledgercore/libs/go/pgxutil"
	httpadapter "github.com/ledgercore/ledgercore/services/identity/internal/adapters/http"
	"github.com/ledgercore/ledgercore/services/identity/internal/adapters/outbox"
	"github.com/ledgercore/ledgercore/services/identity/internal/adapters/postgres"
	"github.com/ledgercore/ledgercore/services/identity/internal/app"
)

const (
	serviceName = "identity"
	schemaName  = "identity"
	defaultAddr = ":8082"
)

type config struct {
	httpAddr      string
	databaseURL   string
	natsURL       string
	adminToken    string
	autoMigrate   bool
	env           string // LEDGERCORE_ENV: dev (default) or sandbox-public
	authDisabled  bool
	signupsPerDay int
	sweepInterval time.Duration
}

func loadConfig() (config, error) {
	addr := os.Getenv("LEDGERCORE_HTTP_ADDR")
	if addr == "" {
		addr = defaultAddr
	}
	env := os.Getenv("LEDGERCORE_ENV")
	if env == "" {
		env = "dev"
	}
	signups := app.DefaultSignupsPerDay
	if v := os.Getenv("LEDGERCORE_SANDBOX_SIGNUPS_PER_DAY"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return config{}, fmt.Errorf("LEDGERCORE_SANDBOX_SIGNUPS_PER_DAY must be a positive integer, got %q", v)
		}
		signups = n
	}
	sweep := time.Hour
	if v := os.Getenv("LEDGERCORE_SANDBOX_SWEEP_INTERVAL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			return config{}, fmt.Errorf("LEDGERCORE_SANDBOX_SWEEP_INTERVAL must be a positive duration (e.g. 1h), got %q", v)
		}
		sweep = d
	}
	return config{
		httpAddr:      addr,
		databaseURL:   os.Getenv("LEDGERCORE_DATABASE_URL"),
		natsURL:       os.Getenv("LEDGERCORE_NATS_URL"),
		adminToken:    os.Getenv("LEDGERCORE_ADMIN_TOKEN"),
		autoMigrate:   os.Getenv("LEDGERCORE_AUTO_MIGRATE") == "true",
		env:           env,
		authDisabled:  os.Getenv("LEDGERCORE_AUTH_DISABLED") == "true",
		signupsPerDay: signups,
		sweepInterval: sweep,
	}, nil
}

// hardeningChecks refuses to start with a dev-grade configuration on a
// public sandbox deployment (fail closed).
func hardeningChecks(cfg config) error {
	if cfg.env != "sandbox-public" {
		return nil
	}
	if cfg.adminToken == "" || cfg.adminToken == "dev-admin-token" {
		return errors.New("refusing to start: LEDGERCORE_ENV=sandbox-public requires a strong LEDGERCORE_ADMIN_TOKEN (empty or the dev default 'dev-admin-token' is forbidden); generate one, e.g. openssl rand -hex 32")
	}
	if cfg.authDisabled {
		return errors.New("refusing to start: LEDGERCORE_AUTH_DISABLED=true is forbidden when LEDGERCORE_ENV=sandbox-public")
	}
	return nil
}

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	if err := run(); err != nil {
		slog.Error("identity service exited with error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	if cfg.databaseURL == "" {
		return errors.New("LEDGERCORE_DATABASE_URL is required")
	}
	if err := hardeningChecks(cfg); err != nil {
		return err
	}
	if cfg.adminToken == "" {
		slog.Warn("LEDGERCORE_ADMIN_TOKEN is not set; admin endpoints are disabled")
	}

	shutdownObs, err := obs.Setup(ctx, serviceName)
	if err != nil {
		return fmt.Errorf("setup observability: %w", err)
	}
	defer shutdownObs()

	pool, err := pgxutil.NewPool(ctx, cfg.databaseURL, schemaName)
	if err != nil {
		return err
	}
	defer pool.Close()

	if cfg.autoMigrate {
		if err := postgres.Migrate(ctx, pool); err != nil {
			return err
		}
		slog.Info("migrations applied", "schema", schemaName)
	}

	store := postgres.NewStore(pool)

	signingKey, err := app.EnsureSigningKey(ctx, store)
	if err != nil {
		return err
	}
	issuer, err := app.NewTokenIssuer(signingKey, app.TokenTTL)
	if err != nil {
		return err
	}

	svc := app.NewService(store, store, store, issuer)

	// Sandbox self-service: public signup endpoint + TTL sweeper. The
	// sweeper writes identity.tenant.expired envelopes to the outbox; the
	// poller drains them to NATS (when configured) so downstream services
	// purge their schemas.
	sandbox := app.NewSandboxService(store, cfg.signupsPerDay)
	go sandbox.RunSweeper(ctx, cfg.sweepInterval)

	if cfg.natsURL != "" {
		nc, err := nats.Connect(cfg.natsURL,
			nats.Name(serviceName),
			nats.MaxReconnects(-1),
			nats.ReconnectWait(2*time.Second),
		)
		if err != nil {
			return fmt.Errorf("connect to NATS at %s: %w", cfg.natsURL, err)
		}
		defer nc.Drain() //nolint:errcheck // best-effort flush on shutdown
		publisher, err := events.NewNATSPublisher(nc)
		if err != nil {
			return err
		}
		go outbox.NewPoller(pool, publisher, outbox.DefaultInterval).Run(ctx)
	} else {
		slog.Info("LEDGERCORE_NATS_URL is empty; identity outbox poller disabled, expiry events stay in the outbox table")
	}

	server := &http.Server{
		Addr:              cfg.httpAddr,
		Handler:           httpadapter.New(svc, pool, cfg.adminToken).WithSandbox(sandbox).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() { errCh <- server.ListenAndServe() }()
	slog.Info("identity service listening", "addr", cfg.httpAddr)

	select {
	case <-ctx.Done():
		slog.Info("shutdown signal received")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}
