// Package logger provides hierarchical logging with levels and formatting.
package logger

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

// Level represents the current logging level.
type Level int

const (
	LevelQuiet Level = iota
	LevelInfo
	LevelVerbose
	LevelDebug
)

// Config holds logger configuration.
type Config struct {
	Level   Level
	LogFile string
	JSON    bool
}

// Logger wraps slog.Logger with additional methods.
type Logger struct {
	*slog.Logger
	config Config
	file   *os.File
}

// New creates a new Logger with the given configuration.
func New(cfg Config) (*Logger, error) {
	var writers []io.Writer
	writers = append(writers, os.Stdout)

	var file *os.File
	if cfg.LogFile != "" {
		dir := filepath.Dir(cfg.LogFile)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create log directory: %w", err)
		}
		var err error
		file, err = os.OpenFile(cfg.LogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return nil, fmt.Errorf("failed to open log file: %w", err)
		}
		writers = append(writers, file)
	}

	// Use a single writer that writes to both stdout and file
	var writer io.Writer
	if len(writers) > 1 {
		writer = io.MultiWriter(writers...)
	} else {
		writer = writers[0]
	}

	var handler slog.Handler
	level := mapLevel(cfg.Level)
	opts := &slog.HandlerOptions{
		Level:     level,
		AddSource: cfg.Level >= LevelDebug,
	}

	if cfg.JSON {
		handler = slog.NewJSONHandler(writer, opts)
	} else {
		handler = slog.NewTextHandler(writer, opts)
	}

	return &Logger{
		Logger: slog.New(handler),
		config: cfg,
		file:   file,
	}, nil
}

// mapLevel converts our Level to slog.Level.
func mapLevel(l Level) slog.Level {
	switch l {
	case LevelQuiet:
		return slog.LevelError
	case LevelInfo:
		return slog.LevelInfo
	case LevelVerbose:
		return slog.LevelDebug
	case LevelDebug:
		return slog.LevelDebug
	default:
		return slog.LevelInfo
	}
}

// IsDebug returns true if debug level is enabled.
func (l *Logger) IsDebug() bool {
	return l.config.Level >= LevelDebug
}

// IsVerbose returns true if verbose level is enabled.
func (l *Logger) IsVerbose() bool {
	return l.config.Level >= LevelVerbose
}

// GetLevel returns the current log level.
func (l *Logger) GetLevel() Level {
	return l.config.Level
}

// Debugf logs a debug message with formatting.
func (l *Logger) Debugf(format string, args ...interface{}) {
	if l.IsDebug() {
		l.Debug(fmt.Sprintf(format, args...))
	}
}

// Verbosef logs a verbose message with formatting.
func (l *Logger) Verbosef(format string, args ...interface{}) {
	if l.IsVerbose() {
		l.Debug(fmt.Sprintf(format, args...))
	}
}

// Infof logs an info message with formatting.
func (l *Logger) Infof(format string, args ...interface{}) {
	l.Info(fmt.Sprintf(format, args...))
}

// Warnf logs a warning message with formatting.
func (l *Logger) Warnf(format string, args ...interface{}) {
	l.Warn(fmt.Sprintf(format, args...))
}

// Errorf logs an error message with formatting.
func (l *Logger) Errorf(format string, args ...interface{}) {
	l.Error(fmt.Sprintf(format, args...))
}

// WithField returns a new logger with a field added.
func (l *Logger) WithField(key string, value interface{}) *Logger {
	return &Logger{
		Logger: l.Logger.With(key, value),
		config: l.config,
		file:   l.file,
	}
}

// WithFields returns a new logger with multiple fields added.
func (l *Logger) WithFields(fields map[string]interface{}) *Logger {
	args := make([]interface{}, 0, len(fields)*2)
	for k, v := range fields {
		args = append(args, k, v)
	}
	return &Logger{
		Logger: l.Logger.With(args...),
		config: l.config,
		file:   l.file,
	}
}

// TimeTrack logs the duration of a function call.
func (l *Logger) TimeTrack(start time.Time, name string) {
	elapsed := time.Since(start)
	if l.IsVerbose() {
		l.Debugf("%s took %v", name, elapsed)
	} else {
		l.Infof("%s took %v", name, elapsed)
	}
}

// Sync flushes any buffered log entries and closes the log file.
func (l *Logger) Sync() error {
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}