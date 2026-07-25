// Command server runs the ledger-core service: the double-entry transactional
// engine of LedgerCore. See README.md for configuration and endpoints.
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

	"github.com/ledgercore/ledgercore/libs/go/events"
	"github.com/ledgercore/ledgercore/libs/go/obs"
	"github.com/ledgercore/ledgercore/libs/go/pgxutil"

	"github.com/ledgercore/ledgercore/services/ledger-core/internal/adapters/http"
	"github.com/ledgercore/ledgercore/services/ledger-core/internal/adapters/outbox"
	"github.com/ledgercore/ledgercore/services/ledger-core/internal/adapters/postgres"
	"github.com/ledgercore/ledgercore/services/ledger-core/internal/app"
)

const (
	serviceName   = "ledger-core"
	serviceSchema = "ledger"
	defaultAddr   = ":8081"
)

type config struct {
	HTTPAddr     string
	DatabaseURL  string
	NATSURL      string
	JWKSURL      string
	AuthDisabled bool
	AutoMigrate  bool
}

func loadConfig() (config, error) {
	cfg := config{
		HTTPAddr:     getenv("LEDGERCORE_HTTP_ADDR", defaultAddr),
		DatabaseURL:  os.Getenv("LEDGERCORE_DATABASE_URL"),
		NATSURL:      os.Getenv("LEDGERCORE_NATS_URL"),
		JWKSURL:      getenv("LEDGERCORE_JWKS_URL", "http://localhost:8082/.well-known/jwks.json"),
		AuthDisabled: os.Getenv("LEDGERCORE_AUTH_DISABLED") == "true",
		AutoMigrate:  os.Getenv("LEDGERCORE_AUTO_MIGRATE") == "true",
	}
	if cfg.DatabaseURL == "" {
		return config{}, errors.New("LEDGERCORE_DATABASE_URL is required")
	}
	return cfg, nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))
	if err := run(); err != nil {
		slog.Error("fatal", "error", err)
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

	shutdownObs, err := obs.Setup(ctx, serviceName)
	if err != nil {
		return fmt.Errorf("observability setup: %w", err)
	}
	defer shutdownObs()

	pool, err := pgxutil.NewPool(ctx, cfg.DatabaseURL, serviceSchema)
	if err != nil {
		return err
	}
	defer pool.Close()

	if cfg.AutoMigrate {
		slog.Info("applying migrations", "schema", serviceSchema)
		if err := postgres.Migrate(ctx, pool); err != nil {
			return err
		}
	}

	store := postgres.NewStore(pool)
	svc := app.NewService(store)

	// Outbox poller: only when NATS is configured. Without it the service
	// still works; events accumulate in the outbox until a poller drains them.
	if cfg.NATSURL != "" {
		nc, err := nats.Connect(cfg.NATSURL,
			nats.Name(serviceName),
			nats.MaxReconnects(-1),
			nats.ReconnectWait(2*time.Second),
		)
		if err != nil {
			return fmt.Errorf("connect to NATS at %s: %w", cfg.NATSURL, err)
		}
		defer nc.Drain() //nolint:errcheck // best-effort flush on shutdown
		publisher, err := events.NewNATSPublisher(nc)
		if err != nil {
			return err
		}
		go outbox.NewPoller(pool, publisher, outbox.DefaultInterval).Run(ctx)
	} else {
		slog.Info("LEDGERCORE_NATS_URL is empty; outbox poller disabled, events stay in the outbox table")
	}

	handler := httpapi.NewRouter(svc, pool.Ping, cfg.JWKSURL, cfg.AuthDisabled)
	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()
	slog.Info("ledger-core listening",
		"addr", cfg.HTTPAddr,
		"auth_disabled", cfg.AuthDisabled,
		"auto_migrate", cfg.AutoMigrate,
	)

	select {
	case <-ctx.Done():
		slog.Info("shutdown signal received")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("http shutdown: %w", err)
		}
		return nil
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
