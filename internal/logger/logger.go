package logger

import (
	"fmt"
	config "github.com/johnny1110/evva/configs"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"
)

// Config holds logger configuration.
// Output is optional — resolved at construction time based on LogDir + AgentID.
type Config struct {
	Level   string
	Format  string
	AgentID string    // required: identifies the agent, used in log filename
	LogDir  string    // optional: if empty, falls back to os.Stdout
	Output  io.Writer // optional: override resolved writer (useful in tests)
}

// New constructs a *slog.Logger.
// Writer resolution priority:
//  1. cfg.Output (explicit override — useful for tests)
//  2. file in cfg.LogDir named "{agentId}+{timestamp}.log"
//  3. os.Stdout
func New(cfg Config) (*slog.Logger, error) {
	writer, err := resolveWriter(cfg)
	if err != nil {
		return nil, fmt.Errorf("logger: resolve writer: %w", err)
	}

	opts := &slog.HandlerOptions{
		Level:     parseLevel(cfg.Level),
		AddSource: true,
	}

	var handler slog.Handler
	if strings.EqualFold(cfg.Format, "json") {
		handler = slog.NewJSONHandler(writer, opts)
	} else {
		handler = slog.NewTextHandler(writer, opts)
	}

	// Bind agentId as a permanent attribute on every log record.
	// slog.Logger.With() returns a child logger that prepends the
	// given attrs to every Handle() call — zero overhead at call sites.
	return slog.New(handler).With("agentId", cfg.AgentID), nil
}

// FromEnv constructs a logger from environment variables.
//
// Env vars:
//
//	LOG_LEVEL   — debug | info | warn | error  (default: info)
//	LOG_FORMAT  — json | text                  (default: text)
//	LOG_DIR     — directory path               (default: stdout)
//	LOG_AGENT_ID — agent identifier            (default: "default")
func FromEnv(agentID string) (*slog.Logger, error) {
	cfg := config.Get()

	if agentID == "" {
		agentID = "default"
	}

	logDir := ""
	if cfg.LogDir != nil {
		logDir = *cfg.LogDir
	}

	return New(Config{
		Level:   cfg.LogLevel,
		Format:  cfg.LogFormat,
		LogDir:  logDir,
		AgentID: agentID,
	})
}

// resolveWriter decides the io.Writer for the logger.
// Separation of concerns: writer resolution is pure I/O policy,
// handler construction is pure formatting policy — keep them apart.
func resolveWriter(cfg Config) (io.Writer, error) {
	// Explicit override wins — supports testing with bytes.Buffer.
	if cfg.Output != nil {
		return cfg.Output, nil
	}

	// No log dir → stdout.
	if cfg.LogDir == "" {
		return os.Stdout, nil
	}

	// MkdirAll is idempotent: no-op if dir already exists,
	// creates the full path (including parents) otherwise.
	if err := os.MkdirAll(cfg.LogDir, 0o755); err != nil {
		return nil, fmt.Errorf("create log dir %q: %w", cfg.LogDir, err)
	}

	filename := buildFilename(cfg.AgentID)
	path := cfg.LogDir + "/" + filename

	// os.O_APPEND is critical: multiple processes with the same agentId
	// (e.g. after a restart) won't truncate the existing log.
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open log file %q: %w", path, err)
	}

	// Fan-out: write to both file and stdout simultaneously.
	// io.MultiWriter delegates Write() to each writer in sequence;
	// if one fails the error is returned immediately.
	return io.MultiWriter(f, os.Stdout), nil
}

// buildFilename produces "{agentId}+{timestamp}.log".
// UTC timestamp avoids timezone ambiguity across distributed nodes.
// Go's reference time: Mon Jan 2 15:04:05 UTC 2006 → layout "20060102T150405Z"
func buildFilename(agentID string) string {
	ts := time.Now().UTC().Format("20060102T150405Z")
	return agentID + "+" + ts + ".log"
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
