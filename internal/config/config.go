// Package config loads and validates control-plane configuration.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds all runtime configuration for the control-plane process.
type Config struct {
	HTTPAddr          string
	XDSAddr           string
	ReconcileInterval time.Duration
	StaleAfter        time.Duration
	ShutdownTimeout   time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	LogLevel          string
}

// Default returns a Config populated with sane local-dev defaults.
func Default() Config {
	return Config{
		HTTPAddr:          ":8080",
		XDSAddr:           ":18000",
		ReconcileInterval: 2 * time.Second,
		StaleAfter:        15 * time.Second,
		ShutdownTimeout:   10 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
		LogLevel:          "info",
	}
}

// Load builds a Config from environment variables, falling back to defaults
// for anything unset.
func Load() (Config, error) {
	cfg := Default()

	if v := os.Getenv("CP_HTTP_ADDR"); v != "" {
		cfg.HTTPAddr = v
	}
	if v := os.Getenv("CP_LOG_LEVEL"); v != "" {
		cfg.LogLevel = v
	}
	if v := os.Getenv("CP_XDS_ADDR"); v != "" {
		cfg.XDSAddr = v
	}
	if v := os.Getenv("CP_RECONCILE_INTERVAL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return Config{}, fmt.Errorf("parse CP_RECONCILE_INTERVAL: %w", err)
		}
		cfg.ReconcileInterval = d
	}
	if v := os.Getenv("CP_STALE_AFTER"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return Config{}, fmt.Errorf("parse CP_STALE_AFTER: %w", err)
		}
		cfg.StaleAfter = d
	}
	if v := os.Getenv("CP_SHUTDOWN_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return Config{}, fmt.Errorf("parse CP_SHUTDOWN_TIMEOUT: %w", err)
		}
		cfg.ShutdownTimeout = d
	}
	if v := os.Getenv("CP_READ_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return Config{}, fmt.Errorf("parse CP_READ_TIMEOUT: %w", err)
		}
		cfg.ReadTimeout = d
	}
	if v := os.Getenv("CP_WRITE_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return Config{}, fmt.Errorf("parse CP_WRITE_TIMEOUT: %w", err)
		}
		cfg.WriteTimeout = d
	}
	if v := os.Getenv("CP_IDLE_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return Config{}, fmt.Errorf("parse CP_IDLE_TIMEOUT: %w", err)
		}
		cfg.IdleTimeout = d
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// Validate checks the configuration for internal consistency, returning an
// error describing the first problem found.
func (c Config) Validate() error {
	if c.HTTPAddr == "" {
		return fmt.Errorf("http addr must not be empty")
	}
	if _, err := strconv.Atoi(portOf(c.HTTPAddr)); err != nil {
		return fmt.Errorf("http addr %q must end in :<port>: %w", c.HTTPAddr, err)
	}
	if c.XDSAddr == "" {
		return fmt.Errorf("xds addr must not be empty")
	}
	if _, err := strconv.Atoi(portOf(c.XDSAddr)); err != nil {
		return fmt.Errorf("xds addr %q must end in :<port>: %w", c.XDSAddr, err)
	}
	if c.ReconcileInterval <= 0 {
		return fmt.Errorf("reconcile interval must be positive, got %s", c.ReconcileInterval)
	}
	if c.StaleAfter <= 0 {
		return fmt.Errorf("stale after must be positive, got %s", c.StaleAfter)
	}
	if c.ShutdownTimeout <= 0 {
		return fmt.Errorf("shutdown timeout must be positive, got %s", c.ShutdownTimeout)
	}
	if c.ReadTimeout <= 0 {
		return fmt.Errorf("read timeout must be positive, got %s", c.ReadTimeout)
	}
	if c.WriteTimeout <= 0 {
		return fmt.Errorf("write timeout must be positive, got %s", c.WriteTimeout)
	}
	if c.IdleTimeout <= 0 {
		return fmt.Errorf("idle timeout must be positive, got %s", c.IdleTimeout)
	}
	switch c.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("log level must be one of debug|info|warn|error, got %q", c.LogLevel)
	}
	return nil
}

// portOf extracts the port substring following the final ':' in addr.
func portOf(addr string) string {
	for i := len(addr) - 1; i >= 0; i-- {
		if addr[i] == ':' {
			return addr[i+1:]
		}
	}
	return addr
}
