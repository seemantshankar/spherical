// Package observability provides logging and OpenTelemetry integration.
package observability

import (
	"context"
	"io"
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/pkgerrors"
)

// Logger wraps zerolog with Knowledge Engine specific functionality.
type Logger struct {
	zl zerolog.Logger
}

// LogConfig holds logger configuration.
type LogConfig struct {
	Level      string
	Format     string // json or console
	Output     io.Writer
	ServiceName string
}

// NewLogger creates a new Logger with the given configuration.
func NewLogger(cfg LogConfig) *Logger {
	// Enable stack traces in errors
	zerolog.ErrorStackMarshaler = pkgerrors.MarshalStack

	// Set log level
	level := parseLevel(cfg.Level)
	zerolog.SetGlobalLevel(level)

	// Choose output
	output := cfg.Output
	if output == nil {
		output = os.Stdout
	}

	// Choose format
	var zl zerolog.Logger
	if cfg.Format == "console" {
		zl = zerolog.New(zerolog.ConsoleWriter{
			Out:        output,
			TimeFormat: time.RFC3339,
		})
	} else {
		zl = zerolog.New(output)
	}

	// Add standard fields
	zl = zl.With().
		Timestamp().
		Str("service", cfg.ServiceName).
		Logger()

	return &Logger{zl: zl}
}

// DefaultLogger returns a logger with default development settings.
func DefaultLogger() *Logger {
	return NewLogger(LogConfig{
		Level:       "debug",
		Format:      "console",
		ServiceName: "knowledge-engine",
	})
}

// With returns a new logger with additional context fields.
func (l *Logger) With() *LoggerContext {
	return &LoggerContext{ctx: l.zl.With()}
}

// Debug logs a debug message.
func (l *Logger) Debug() *LogEvent {
	return &LogEvent{evt: l.zl.Debug()}
}

// Info logs an info message.
func (l *Logger) Info() *LogEvent {
	return &LogEvent{evt: l.zl.Info()}
}

// Warn logs a warning message.
func (l *Logger) Warn() *LogEvent {
	return &LogEvent{evt: l.zl.Warn()}
}

// Error logs an error message.
func (l *Logger) Error() *LogEvent {
	return &LogEvent{evt: l.zl.Error()}
}

// Fatal logs a fatal message and exits.
func (l *Logger) Fatal() *LogEvent {
	return &LogEvent{evt: l.zl.Fatal()}
}

// WithContext returns a logger with request context (trace ID, etc.).
func (l *Logger) WithContext(ctx context.Context) *Logger {
	// Extract trace ID if present
	if traceID := TraceIDFromContext(ctx); traceID != "" {
		return &Logger{zl: l.zl.With().Str("trace_id", traceID).Logger()}
	}
	return l
}

// WithTenant returns a logger with tenant context.
func (l *Logger) WithTenant(tenantID string) *Logger {
	return &Logger{zl: l.zl.With().Str("tenant_id", tenantID).Logger()}
}

// WithOperation returns a logger with operation context.
func (l *Logger) WithOperation(op string) *Logger {
	return &Logger{zl: l.zl.With().Str("operation", op).Logger()}
}

// LoggerContext builds a new logger with context.
type LoggerContext struct {
	ctx zerolog.Context
}

// Str adds a string field.
func (c *LoggerContext) Str(key, val string) *LoggerContext {
	c.ctx = c.ctx.Str(key, val)
	return c
}

// Int adds an int field.
func (c *LoggerContext) Int(key string, val int) *LoggerContext {
	c.ctx = c.ctx.Int(key, val)
	return c
}

// Bool adds a bool field.
func (c *LoggerContext) Bool(key string, val bool) *LoggerContext {
	c.ctx = c.ctx.Bool(key, val)
	return c
}

// Dur adds a duration field.
func (c *LoggerContext) Dur(key string, val time.Duration) *LoggerContext {
	c.ctx = c.ctx.Dur(key, val)
	return c
}

// Logger returns the configured logger.
func (c *LoggerContext) Logger() *Logger {
	return &Logger{zl: c.ctx.Logger()}
}

// LogEvent represents a log event being built.
type LogEvent struct {
	evt *zerolog.Event
}

// Str adds a string field.
func (e *LogEvent) Str(key, val string) *LogEvent {
	e.evt = e.evt.Str(key, val)
	return e
}

// Int adds an int field.
func (e *LogEvent) Int(key string, val int) *LogEvent {
	e.evt = e.evt.Int(key, val)
	return e
}

// Int64 adds an int64 field.
func (e *LogEvent) Int64(key string, val int64) *LogEvent {
	e.evt = e.evt.Int64(key, val)
	return e
}

// Float64 adds a float64 field.
func (e *LogEvent) Float64(key string, val float64) *LogEvent {
	e.evt = e.evt.Float64(key, val)
	return e
}

// Bool adds a bool field.
func (e *LogEvent) Bool(key string, val bool) *LogEvent {
	e.evt = e.evt.Bool(key, val)
	return e
}

// Strs adds a string slice field.
func (e *LogEvent) Strs(key string, val []string) *LogEvent {
	e.evt = e.evt.Strs(key, val)
	return e
}

// Dur adds a duration field.
func (e *LogEvent) Dur(key string, val time.Duration) *LogEvent {
	e.evt = e.evt.Dur(key, val)
	return e
}

// Err adds an error field.
func (e *LogEvent) Err(err error) *LogEvent {
	e.evt = e.evt.Err(err)
	return e
}

// Interface adds any value as a field.
func (e *LogEvent) Interface(key string, val interface{}) *LogEvent {
	e.evt = e.evt.Interface(key, val)
	return e
}

// Msg sends the log event with a message.
func (e *LogEvent) Msg(msg string) {
	e.evt.Msg(msg)
}

// Msgf sends the log event with a formatted message.
func (e *LogEvent) Msgf(format string, args ...interface{}) {
	e.evt.Msgf(format, args...)
}

// Send sends the log event without a message.
func (e *LogEvent) Send() {
	e.evt.Send()
}

// parseLevel converts a string level to zerolog.Level.
func parseLevel(level string) zerolog.Level {
	switch level {
	case "trace":
		return zerolog.TraceLevel
	case "debug":
		return zerolog.DebugLevel
	case "info":
		return zerolog.InfoLevel
	case "warn", "warning":
		return zerolog.WarnLevel
	case "error":
		return zerolog.ErrorLevel
	case "fatal":
		return zerolog.FatalLevel
	case "panic":
		return zerolog.PanicLevel
	default:
		return zerolog.InfoLevel
	}
}

// Context keys for tracing.
type contextKey string

const traceIDKey contextKey = "trace_id"

// ContextWithTraceID adds a trace ID to the context.
func ContextWithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, traceIDKey, traceID)
}

// TraceIDFromContext extracts a trace ID from the context.
func TraceIDFromContext(ctx context.Context) string {
	if v := ctx.Value(traceIDKey); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

