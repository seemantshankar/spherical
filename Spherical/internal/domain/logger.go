package domain

import (
	"fmt"
	"log"
	"os"
)

// LogLevel represents logging severity
type LogLevel int

const (
	LogLevelDebug LogLevel = iota
	LogLevelInfo
	LogLevelWarn
	LogLevelError
)

// Logger provides structured logging
type Logger struct {
	level  LogLevel
	logger *log.Logger
}

// NewLogger creates a new logger instance
func NewLogger(level LogLevel) *Logger {
	return &Logger{
		level:  level,
		logger: log.New(os.Stdout, "", log.LstdFlags),
	}
}

// Debug logs debug-level messages
func (l *Logger) Debug(format string, v ...interface{}) {
	if l.level <= LogLevelDebug {
		l.logger.Printf("[DEBUG] "+format, v...)
	}
}

// Info logs info-level messages
func (l *Logger) Info(format string, v ...interface{}) {
	if l.level <= LogLevelInfo {
		l.logger.Printf("[INFO] "+format, v...)
	}
}

// Warn logs warning-level messages
func (l *Logger) Warn(format string, v ...interface{}) {
	if l.level <= LogLevelWarn {
		l.logger.Printf("[WARN] "+format, v...)
	}
}

// Error logs error-level messages
func (l *Logger) Error(format string, v ...interface{}) {
	if l.level <= LogLevelError {
		l.logger.Printf("[ERROR] "+format, v...)
	}
}

// Fatal logs fatal error and exits
func (l *Logger) Fatal(format string, v ...interface{}) {
	l.logger.Fatalf("[FATAL] "+format, v...)
}

// WithPrefix returns a new logger with a prefix
func (l *Logger) WithPrefix(prefix string) *Logger {
	return &Logger{
		level:  l.level,
		logger: log.New(os.Stdout, fmt.Sprintf("[%s] ", prefix), log.LstdFlags),
	}
}

// DefaultLogger is the default logger instance
var DefaultLogger = NewLogger(LogLevelInfo)




