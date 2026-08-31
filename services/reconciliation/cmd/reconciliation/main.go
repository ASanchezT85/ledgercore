// Command reconciliation runs the LedgerCore reconciliation service:
// HTTP API on :8083, a JetStream consumer that mirrors ledger postings, and
// the transactional outbox poller for recon.discrepancy.detected events.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ASanchezT85/ledgercore/libs/go/events"
	"github.com/ASanchezT85/ledgercore/libs/go/obs"
	"github.com/ASanchezT85/ledgercore/libs/go/pgxutil"
	"github.com/nats-io/nats.go"

	"github.com/ASanchezT85/ledgercore/services/reconciliation/internal/adapters/httpapi"
	"github.com/ASanchezT85/ledgercore/services/reconciliation/internal/adapters/natsconsumer"
	"github.com/ASanchezT85/ledgercore/services/reconciliation/internal/adapters/postgres"
	"github.com/ASanchezT85/ledgercore/services/reconciliation/internal/adapters/purge"
	"github.com/ASanchezT85/ledgercore/services/reconciliation/internal/app"
	"github.com/ASanchezT85/ledgercore/services/reconciliation/internal/config"
	"github.com/ASanchezT85/ledgercore/services/reconciliation/internal/outbox"
)

const serviceName = "reconciliation"

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("service", serviceName)
	slog.SetDefault(logger)

	if err := run(logger); err != nil {
		logger.Error("service exited with error", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	shutdownObs, err := obs.Setup(ctx, serviceName)
	if err != nil {
		return err
	}
	defer shutdownObs()

	if cfg.AutoMigrate {
		logger.Info("running migrations (LEDGERCORE_AUTO_MIGRATE=true)")
		if err := postgres.Migrate(ctx, cfg.DatabaseURL); err != nil {
			return err
		}
	}

	pool, err := pgxutil.NewPool(ctx, cfg.DatabaseURL, "recon")
	if err != nil {
		return err
	}
	defer pool.Close()

	store := postgres.NewStore(pool)
	svc := app.NewService(store, logger)

	// NATS is optional in dev: without it the service still serves the API,
	// but the ledger mirror stays frozen and events stay in the outbox.
	if cfg.NATSURL != "" {
		nc, err := nats.Connect(cfg.NATSURL,
			nats.Name(serviceName),
			nats.MaxReconnects(-1),
			nats.ReconnectWait(2*time.Second),
		)
		if err != nil {
			return err
		}
		defer nc.Drain() //nolint:errcheck // best-effort drain on shutdown

		// NewNATSPublisher also ensures the LEDGERCORE stream exists, which
		// the durable consumer below binds to.
		pub, err := events.NewNATSPublisher(nc)
		if err != nil {
			return err
		}

		sub, err := natsconsumer.Start(ctx, nc, store, logger)
		if err != nil {
			return err
		}
		defer sub.Unsubscribe() //nolint:errcheck // best-effort on shutdown

		// Sandbox TTL: purge this schema when identity announces an
		// expired tenant.
		purgeSub, err := purge.Start(ctx, nc, pool)
		if err != nil {
			return err
		}
		defer purgeSub.Unsubscribe() //nolint:errcheck // best-effort on shutdown

		go outbox.Run(ctx, store, pub, outbox.DefaultInterval, logger)
	} else {
		logger.Warn("LEDGERCORE_NATS_URL is empty: mirror consumer and outbox poller are disabled")
	}

	handler := httpapi.NewHandler(svc, pool, cfg.JWKSURL, cfg.AuthDisabled)
	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("http server listening", "addr", cfg.HTTPAddr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	logger.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return server.Shutdown(shutdownCtx)
}
