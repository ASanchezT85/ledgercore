// Package config loads the service configuration from LEDGERCORE_* env vars.
package config

import (
	"errors"
	"os"
	"strconv"
)

// Config is the runtime configuration of the reconciliation service.
type Config struct {
	HTTPAddr     string // LEDGERCORE_HTTP_ADDR, default ":8083"
	DatabaseURL  string // LEDGERCORE_DATABASE_URL, required
	NATSURL      string // LEDGERCORE_NATS_URL, empty disables consumer+poller
	JWKSURL      string // LEDGERCORE_JWKS_URL, identity JWKS endpoint
	AuthDisabled bool   // LEDGERCORE_AUTH_DISABLED, dev only
	AutoMigrate  bool   // LEDGERCORE_AUTO_MIGRATE
}

// Load reads the environment. It fails fast when required values are missing
// or inconsistent (JWKS is required unless auth is disabled).
func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:     getenv("LEDGERCORE_HTTP_ADDR", ":8083"),
		DatabaseURL:  os.Getenv("LEDGERCORE_DATABASE_URL"),
		NATSURL:      os.Getenv("LEDGERCORE_NATS_URL"),
		JWKSURL:      os.Getenv("LEDGERCORE_JWKS_URL"),
		AuthDisabled: boolenv("LEDGERCORE_AUTH_DISABLED"),
		AutoMigrate:  boolenv("LEDGERCORE_AUTO_MIGRATE"),
	}
	if cfg.DatabaseURL == "" {
		return Config{}, errors.New("config: LEDGERCORE_DATABASE_URL is required")
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

func boolenv(key string) bool {
	v, err := strconv.ParseBool(os.Getenv(key))
	return err == nil && v
}
