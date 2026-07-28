// Command webhooks runs the LedgerCore webhooks service: the signed webhook
// dispatcher that fans platform events out to customer endpoints.
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

	"github.com/nats-io/nats.go"

	"github.com/ledgercore/ledgercore/libs/go/obs"
	"github.com/ledgercore/ledgercore/libs/go/pgxutil"
	"github.com/ledgercore/ledgercore/services/webhooks/internal/adapters/httpapi"
	"github.com/ledgercore/ledgercore/services/webhooks/internal/adapters/natsconsumer"
	"github.com/ledgercore/ledgercore/services/webhooks/internal/adapters/postgres"
	"github.com/ledgercore/ledgercore/services/webhooks/internal/adapters/purge"
	"github.com/ledgercore/ledgercore/services/webhooks/internal/app"
	"github.com/ledgercore/ledgercore/services/webhooks/internal/config"
	"github.com/ledgercore/ledgercore/services/webhooks/internal/dispatcher"
)

const serviceName = "webhooks"

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	if err := run(); err != nil {
		slog.Error("service exited with error", "service", serviceName, "error", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	shutdownObs, err := obs.Setup(ctx, serviceName)
	if err != nil {
		return fmt.Errorf("observability setup: %w", err)
	}
	defer shutdownObs()

	pool, err := pgxutil.NewPool(ctx, cfg.DatabaseURL, postgres.SchemaName)
	if err != nil {
		return err
	}
	defer pool.Close()

	if cfg.AutoMigrate {
		if err := postgres.Migrate(ctx, pool); err != nil {
			return err
		}
		slog.Info("migrations applied", "schema", postgres.SchemaName)
	}

	repo := postgres.NewRepo(pool)
	svc := app.NewService(repo, repo)

	consumer := natsconsumer.New(cfg.NATSURL, svc)
	consumer.Start(ctx)
	defer consumer.Close()

	// Sandbox TTL: purge this schema when identity announces an expired
	// tenant. Own connection with background retry, mirroring the fan-out
	// consumer's tolerance to a NATS outage at startup.
	go func() {
		for {
			nc, err := nats.Connect(cfg.NATSURL,
				nats.Name(serviceName+"-purge"),
				nats.MaxReconnects(-1),
				nats.ReconnectWait(2*time.Second),
			)
			if err == nil {
				if _, err = purge.Start(ctx, nc, pool); err == nil {
					return
				}
				nc.Close()
			}
			slog.Warn("webhooks purge consumer setup failed; retrying",
				"url", cfg.NATSURL, "retry_in", "5s", "error", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
			}
		}
	}()

	disp := dispatcher.New(repo)
	go disp.Run(ctx)

	// Sweep expired previous secrets (rotation grace window, 24h) hourly so
	// old secrets never outlive their window.
	go func() {
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if n, err := repo.PurgeExpiredPreviousSecrets(ctx); err != nil {
					slog.Error("purge expired previous secrets failed", "error", err)
				} else if n > 0 {
					slog.Info("purged expired previous webhook secrets", "count", n)
				}
			}
		}
	}()

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           httpapi.NewHandler(svc, pool, cfg),
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		slog.Info("http server listening", "service", serviceName, "addr", cfg.HTTPAddr)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		slog.Info("shutdown signal received")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
