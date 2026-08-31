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

	"github.com/ASanchezT85/ledgercore/libs/go/events"
	"github.com/ASanchezT85/ledgercore/libs/go/obs"
	"github.com/ASanchezT85/ledgercore/libs/go/pgxutil"
	httpadapter "github.com/ASanchezT85/ledgercore/services/identity/internal/adapters/http"
	"github.com/ASanchezT85/ledgercore/services/identity/internal/adapters/outbox"
	"github.com/ASanchezT85/ledgercore/services/identity/internal/adapters/postgres"
	"github.com/ASanchezT85/ledgercore/services/identity/internal/app"
	"github.com/ASanchezT85/ledgercore/services/identity/internal/keycrypt"
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
	masterKey     string // LEDGERCORE_MASTER_KEY: 32-byte hex; encrypts signing keys at rest
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
		masterKey:     os.Getenv("LEDGERCORE_MASTER_KEY"),
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
	if cfg.masterKey == "" {
		return errors.New("refusing to start: LEDGERCORE_ENV=sandbox-public requires LEDGERCORE_MASTER_KEY (32-byte hex) to encrypt signing keys at rest; generate one, e.g. openssl rand -hex 32")
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

	var masterCipher *keycrypt.Cipher
	if cfg.masterKey != "" {
		masterCipher, err = keycrypt.New(cfg.masterKey)
		if err != nil {
			return err
		}
	} else {
		slog.Warn("LEDGERCORE_MASTER_KEY is not set; signing keys are stored in PLAINTEXT (dev only — forbidden in sandbox-public)")
	}

	signingKey, err := app.EnsureSigningKey(ctx, store, masterCipher)
	if err != nil {
		return err
	}
	issuer, err := app.NewTokenIssuer(signingKey, app.TokenTTL)
	if err != nil {
		return err
	}

	svc := app.NewService(store, store, store, issuer)

	// Sandbox self-service: an UNAUTHENTICATED public signup endpoint plus a
	// TTL sweeper that HARD-DELETES each tenant it created 14 days later,
	// propagating the purge to the other services over NATS.
	//
	// Both are opt-in, and deliberately so. They exist to run a throwaway
	// public trial, and either one is destructive or dangerous in a normal
	// self-hosted deployment: the endpoint lets anyone on the network create a
	// tenant, and the sweeper deletes ledger data on a timer. A self-hosted
	// install creates tenants through identity's admin API instead.
	//
	// LEDGERCORE_ENV=sandbox-public is the only thing that turns them on, and
	// that value already forces a strong admin token, real auth and a master
	// key (see requireHardenedSandbox above).
	var sandbox *app.SandboxService
	if cfg.env == "sandbox-public" {
		sandbox = app.NewSandboxService(store, cfg.signupsPerDay)
		go sandbox.RunSweeper(ctx, cfg.sweepInterval)
		slog.Warn("public sandbox mode: unauthenticated signups are enabled and tenants expire",
			"ttl", app.SandboxTTL, "sweep_interval", cfg.sweepInterval)
	}

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

	// WithSandbox only when the service actually exists. Passing a nil
	// *app.SandboxService through the interface parameter would produce a
	// non-nil interface holding a nil pointer, and the server's `!= nil`
	// check would register the endpoint and then panic on the first request.
	api := httpadapter.New(svc, pool, cfg.adminToken)
	if sandbox != nil {
		api = api.WithSandbox(sandbox)
	}

	server := &http.Server{
		Addr:              cfg.httpAddr,
		Handler:           api.Handler(),
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
