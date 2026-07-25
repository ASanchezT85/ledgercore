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
	"syscall"
	"time"

	"github.com/ledgercore/ledgercore/libs/go/obs"
	"github.com/ledgercore/ledgercore/libs/go/pgxutil"
	httpadapter "github.com/ledgercore/ledgercore/services/identity/internal/adapters/http"
	"github.com/ledgercore/ledgercore/services/identity/internal/adapters/postgres"
	"github.com/ledgercore/ledgercore/services/identity/internal/app"
)

const (
	serviceName = "identity"
	schemaName  = "identity"
	defaultAddr = ":8082"
)

type config struct {
	httpAddr    string
	databaseURL string
	adminToken  string
	autoMigrate bool
}

func loadConfig() config {
	addr := os.Getenv("LEDGERCORE_HTTP_ADDR")
	if addr == "" {
		addr = defaultAddr
	}
	return config{
		httpAddr:    addr,
		databaseURL: os.Getenv("LEDGERCORE_DATABASE_URL"),
		adminToken:  os.Getenv("LEDGERCORE_ADMIN_TOKEN"),
		autoMigrate: os.Getenv("LEDGERCORE_AUTO_MIGRATE") == "true",
	}
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

	cfg := loadConfig()
	if cfg.databaseURL == "" {
		return errors.New("LEDGERCORE_DATABASE_URL is required")
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
	server := &http.Server{
		Addr:              cfg.httpAddr,
		Handler:           httpadapter.New(svc, pool, cfg.adminToken).Handler(),
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
