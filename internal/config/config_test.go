package config

import "testing"

func TestDefaultIsValid(t *testing.T) {
	if err := Default().Validate(); err != nil {
		t.Fatalf("default config should be valid: %v", err)
	}
}

func TestValidateRejectsBadAddr(t *testing.T) {
	cfg := Default()
	cfg.HTTPAddr = "not-an-addr"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for invalid http addr")
	}
}

func TestValidateRejectsNonPositiveTimeouts(t *testing.T) {
	cases := []func(*Config){
		func(c *Config) { c.ShutdownTimeout = 0 },
		func(c *Config) { c.ReadTimeout = -1 },
		func(c *Config) { c.WriteTimeout = 0 },
		func(c *Config) { c.IdleTimeout = 0 },
	}
	for _, mutate := range cases {
		cfg := Default()
		mutate(&cfg)
		if err := cfg.Validate(); err == nil {
			t.Fatalf("expected validation error, config=%+v", cfg)
		}
	}
}

func TestValidateRejectsBadLogLevel(t *testing.T) {
	cfg := Default()
	cfg.LogLevel = "verbose"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for invalid log level")
	}
}

func TestLoadUsesEnvOverrides(t *testing.T) {
	t.Setenv("CP_HTTP_ADDR", ":9090")
	t.Setenv("CP_LOG_LEVEL", "debug")
	t.Setenv("CP_SHUTDOWN_TIMEOUT", "2s")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.HTTPAddr != ":9090" {
		t.Errorf("HTTPAddr = %q, want :9090", cfg.HTTPAddr)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want debug", cfg.LogLevel)
	}
	if cfg.ShutdownTimeout.String() != "2s" {
		t.Errorf("ShutdownTimeout = %s, want 2s", cfg.ShutdownTimeout)
	}
}

func TestLoadRejectsBadDuration(t *testing.T) {
	t.Setenv("CP_READ_TIMEOUT", "not-a-duration")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for malformed duration env var")
	}
}
