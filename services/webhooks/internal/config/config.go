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
