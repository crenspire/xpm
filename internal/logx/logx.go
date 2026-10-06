// Package logx provides logging utilities for xpm.
//
// The package supports multiple log levels, optional structured logging,
// and configurable output destinations including file output.
//
// Example usage:
//
//	logx.SetLevel(logx.LevelDebug)
//	logx.Debug("Processing package %s", pkg)
//	logx.Info("Installation complete")
//	logx.Warn("Deprecated API used")
//	logx.Error("Failed to connect: %v", err)
package logx

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

// Level represents a log level.
type Level int

// Log levels from most to least verbose.
const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
	LevelNone
)

// String returns the string representation of a log level.
func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// Config holds logger configuration.
type Config struct {
	// Level is the minimum log level to output.
	Level Level
	// Output is the destination for log messages.
	Output io.Writer
	// ShowTimestamp controls whether timestamps are shown.
	ShowTimestamp bool
	// ShowLevel controls whether log levels are shown.
	ShowLevel bool
	// Prefix is prepended to all log messages.
	Prefix string
	// Structured enables JSON-structured logging.
	Structured bool
}

// DefaultConfig returns the default logger configuration.
func DefaultConfig() Config {
	return Config{
		Level:         LevelInfo,
		Output:        os.Stderr,
		ShowTimestamp: false,
		ShowLevel:     true,
		Prefix:        "[xpm]",
		Structured:    false,
	}
}

// logger is the global logger state.
var (
	mu     sync.Mutex
	config = DefaultConfig()

	// Verbose is a backward-compatible flag for enabling debug logging.
	// Deprecated: Use SetLevel(LevelDebug) instead.
	Verbose = false
)

// SetConfig sets the logger configuration.
func SetConfig(cfg Config) {
	mu.Lock()
	defer mu.Unlock()
	config = cfg
}

// GetConfig returns the current logger configuration.
func GetConfig() Config {
	mu.Lock()
	defer mu.Unlock()
	return config
}

// SetLevel sets the minimum log level.
func SetLevel(level Level) {
	mu.Lock()
	defer mu.Unlock()
	config.Level = level
}

// SetOutput sets the log output destination.
func SetOutput(w io.Writer) {
	mu.Lock()
	defer mu.Unlock()
	config.Output = w
}

// SetPrefix sets the log prefix.
func SetPrefix(prefix string) {
	mu.Lock()
	defer mu.Unlock()
	config.Prefix = prefix
}

// EnableTimestamps enables timestamp display in log messages.
func EnableTimestamps() {
	mu.Lock()
	defer mu.Unlock()
	config.ShowTimestamp = true
}

// EnableStructured enables JSON-structured logging.
func EnableStructured() {
	mu.Lock()
	defer mu.Unlock()
	config.Structured = true
}

// SetLogFile sets log output to a file.
// Returns a function to close the file.
func SetLogFile(path string) (func() error, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}

	mu.Lock()
	config.Output = file
	mu.Unlock()

	return file.Close, nil
}

// log writes a log message at the specified level.
func log(level Level, format string, args ...interface{}) {
	mu.Lock()
	cfg := config
	verbose := Verbose
	mu.Unlock()

	// For backward compatibility: Info level requires Verbose flag
	// unless the config level is explicitly set to LevelInfo or lower
	if level == LevelInfo && !verbose {
		return
	}

	// Check configured level for other log levels
	if level < cfg.Level {
		return
	}

	msg := fmt.Sprintf(format, args...)

	if cfg.Structured {
		writeStructured(cfg, level, msg)
	} else {
		writePlain(cfg, level, msg)
	}
}

// writePlain writes a plain-text log message.
func writePlain(cfg Config, level Level, msg string) {
	var parts []string

	if cfg.ShowTimestamp {
		parts = append(parts, time.Now().Format("2006-01-02 15:04:05"))
	}

	if cfg.ShowLevel {
		parts = append(parts, fmt.Sprintf("%-5s", level.String()))
	}

	if cfg.Prefix != "" {
		parts = append(parts, cfg.Prefix)
	}

	parts = append(parts, msg)

	line := strings.Join(parts, " ") + "\n"
	_, _ = fmt.Fprint(cfg.Output, line)
}

// writeStructured writes a JSON-structured log message.
func writeStructured(cfg Config, level Level, msg string) {
	timestamp := time.Now().Format(time.RFC3339)
	// Simple JSON without external dependencies
	escaped := strings.ReplaceAll(msg, `"`, `\"`)
	escaped = strings.ReplaceAll(escaped, "\n", "\\n")

	line := fmt.Sprintf(`{"time":"%s","level":"%s","msg":"%s"}%s`,
		timestamp, level.String(), escaped, "\n")
	_, _ = fmt.Fprint(cfg.Output, line)
}

// Debug logs a debug message.
func Debug(format string, args ...interface{}) {
	log(LevelDebug, format, args...)
}

// Info logs an informational message.
// This is the primary logging function, compatible with the original API.
func Info(format string, args ...interface{}) {
	log(LevelInfo, format, args...)
}

// Warn logs a warning message.
func Warn(format string, args ...interface{}) {
	log(LevelWarn, format, args...)
}

// Error logs an error message.
func Error(format string, args ...interface{}) {
	log(LevelError, format, args...)
}

// Fatal logs an error message and exits with code 1.
func Fatal(format string, args ...interface{}) {
	log(LevelError, format, args...)
	os.Exit(1)
}

// WithFields returns a FieldLogger for structured logging.
func WithFields(fields map[string]interface{}) *FieldLogger {
	return &FieldLogger{fields: fields}
}

// FieldLogger provides structured logging with fields.
type FieldLogger struct {
	fields map[string]interface{}
}

// Debug logs a debug message with fields.
func (l *FieldLogger) Debug(format string, args ...interface{}) {
	l.log(LevelDebug, format, args...)
}

// Info logs an info message with fields.
func (l *FieldLogger) Info(format string, args ...interface{}) {
	l.log(LevelInfo, format, args...)
}

// Warn logs a warning message with fields.
func (l *FieldLogger) Warn(format string, args ...interface{}) {
	l.log(LevelWarn, format, args...)
}

// Error logs an error message with fields.
func (l *FieldLogger) Error(format string, args ...interface{}) {
	l.log(LevelError, format, args...)
}

func (l *FieldLogger) log(level Level, format string, args ...interface{}) {
	mu.Lock()
	cfg := config
	mu.Unlock()

	if level < cfg.Level {
		return
	}

	msg := fmt.Sprintf(format, args...)

	if cfg.Structured {
		l.writeStructuredWithFields(cfg, level, msg)
	} else {
		// For plain text, append fields as key=value
		var fieldParts []string
		for k, v := range l.fields {
			fieldParts = append(fieldParts, fmt.Sprintf("%s=%v", k, v))
		}
		if len(fieldParts) > 0 {
			msg = msg + " " + strings.Join(fieldParts, " ")
		}
		writePlain(cfg, level, msg)
	}
}

func (l *FieldLogger) writeStructuredWithFields(cfg Config, level Level, msg string) {
	timestamp := time.Now().Format(time.RFC3339)
	escaped := strings.ReplaceAll(msg, `"`, `\"`)
	escaped = strings.ReplaceAll(escaped, "\n", "\\n")

	// Build fields JSON
	var fieldParts []string
	for k, v := range l.fields {
		valStr := fmt.Sprintf("%v", v)
		valStr = strings.ReplaceAll(valStr, `"`, `\"`)
		fieldParts = append(fieldParts, fmt.Sprintf(`"%s":"%s"`, k, valStr))
	}
	fieldsJSON := strings.Join(fieldParts, ",")

	line := fmt.Sprintf(`{"time":"%s","level":"%s","msg":"%s",%s}%s`,
		timestamp, level.String(), escaped, fieldsJSON, "\n")
	_, _ = fmt.Fprint(cfg.Output, line)
}
