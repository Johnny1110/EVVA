package config

import (
	"os"
	"strings"
	"sync"
	"time"
)

// AppConfig holds all parsed environment configuration.
// Fields are read-only after initialization — treat them as constants.
// Pointer types (e.g. *string) represent "explicitly nullable" values:
// nil means "not set / intentionally absent", distinguishing from "".
type AppConfig struct {
	// Logging
	LogLevel  string  // default: "info"
	LogFormat string  // default: "text"
	LogDir    *string // default: nil → stdout only

	// Application
	AppEnv  string // default: "development"
	AppName string // default: "app"

	// Loaded metadata
	LoadedAt time.Time
}

var (
	instance *AppConfig
	once     sync.Once
)

// Get returns the singleton AppConfig, initializing it on first call.
// Safe for concurrent use — subsequent calls after the first are lock-free reads.
func Get() *AppConfig {
	once.Do(func() {
		instance = load()
	})
	return instance
}

// load performs the actual env parsing.
// Isolated from Get() so it's independently testable:
// call load() directly in tests without touching the singleton.
func load() *AppConfig {
	cfg := &AppConfig{
		LogLevel:  getEnvDefault("LOG_LEVEL", "info"),
		LogFormat: getEnvDefault("LOG_FORMAT", "text"),
		LogDir:    getEnvNullable("LOG_DIR"),

		AppEnv:  getEnvDefault("APP_ENV", "dev"),
		AppName: getEnvDefault("APP_NAME", "evva"),

		LoadedAt: time.Now().UTC(),
	}

	// Normalize: lowercase for comparison safety downstream
	cfg.LogLevel = strings.ToLower(cfg.LogLevel)
	cfg.LogFormat = strings.ToLower(cfg.LogFormat)
	cfg.AppEnv = strings.ToLower(cfg.AppEnv)

	return cfg
}

// getEnvDefault returns the env var value, or fallback if unset/empty.
// Uses LookupEnv to distinguish "unset" from "set to empty string";
// both are treated as "use default" here — empty string is not a valid value
// for config fields like LOG_LEVEL.
func getEnvDefault(key, fallback string) string {
	val, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(val) == "" {
		return fallback
	}
	return strings.TrimSpace(val)
}

// getEnvNullable returns nil if the var is unset or empty,
// or a pointer to the trimmed value if present.
// This preserves the semantic distinction:
//
//	nil   → "not configured, use default behavior"
//	&""   → never returned (empty treated as nil)
//	&"/var/log" → explicitly configured
func getEnvNullable(key string) *string {
	val, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(val) == "" {
		return nil
	}
	trimmed := strings.TrimSpace(val)
	return &trimmed
}

// IsDevelopment / IsProduction — semantic helpers so call sites
// don't hardcode string literals scattered across the codebase.
func (c *AppConfig) IsDevelopment() bool { return c.AppEnv == "dev" }
func (c *AppConfig) IsProduction() bool  { return c.AppEnv == "prod" }
