// Package config loads the service configuration from LEDGERCORE_* env vars.
package config

import (
	"errors"
	"os"
)

// Config holds every runtime setting of the webhooks service.
type Config struct {
	HTTPAddr     string
	DatabaseURL  string
	NATSURL      string
	JWKSURL      string
	AuthDisabled bool
	AutoMigrate  bool
	Env          string // LEDGERCORE_ENV: dev (default) or sandbox-public
	MasterKey    string // LEDGERCORE_MASTER_KEY: 32-byte hex; encrypts signing secrets at rest
}

// Load reads the environment. It returns an error when a required variable
// is missing so main can fail fast with a clear message.
func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:     getenv("LEDGERCORE_HTTP_ADDR", ":8084"),
		DatabaseURL:  os.Getenv("LEDGERCORE_DATABASE_URL"),
		NATSURL:      getenv("LEDGERCORE_NATS_URL", "nats://localhost:4222"),
		JWKSURL:      os.Getenv("LEDGERCORE_JWKS_URL"),
		AuthDisabled: os.Getenv("LEDGERCORE_AUTH_DISABLED") == "true",
		AutoMigrate:  os.Getenv("LEDGERCORE_AUTO_MIGRATE") == "true",
		Env:          getenv("LEDGERCORE_ENV", "dev"),
		MasterKey:    os.Getenv("LEDGERCORE_MASTER_KEY"),
	}
	if cfg.DatabaseURL == "" {
		return Config{}, errors.New("config: LEDGERCORE_DATABASE_URL is required")
	}
	// Fail closed on public sandbox deployments.
	if cfg.Env == "sandbox-public" && cfg.AuthDisabled {
		return Config{}, errors.New("config: refusing to start: LEDGERCORE_AUTH_DISABLED=true is forbidden when LEDGERCORE_ENV=sandbox-public")
	}
	// Webhook signing secrets must be encrypted at rest in public sandbox.
	if cfg.Env == "sandbox-public" && cfg.MasterKey == "" {
		return Config{}, errors.New("config: refusing to start: LEDGERCORE_ENV=sandbox-public requires LEDGERCORE_MASTER_KEY (32-byte hex) to encrypt webhook secrets at rest; generate one, e.g. openssl rand -hex 32")
	}
	if !cfg.AuthDisabled && cfg.JWKSURL == "" {
		return Config{}, errors.New("config: LEDGERCORE_JWKS_URL is required unless LEDGERCORE_AUTH_DISABLED=true")
	}
	return cfg, nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
