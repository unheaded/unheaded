// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package logger

import (
	"context"
	"io"
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// contextKey is the key for logger in context
type contextKey struct{}

var loggerKey = contextKey{}

// Config holds logger configuration
type Config struct {
	Level      string // debug, info, warn, error
	Pretty     bool   // Human-readable output for development
	Output     io.Writer
	WithCaller bool // Include file:line in logs
}

// Initialize sets up the global logger
func Initialize(cfg Config) {
	// Set log level
	level := zerolog.InfoLevel
	switch cfg.Level {
	case "debug":
		level = zerolog.DebugLevel
	case "info":
		level = zerolog.InfoLevel
	case "warn":
		level = zerolog.WarnLevel
	case "error":
		level = zerolog.ErrorLevel
	}
	zerolog.SetGlobalLevel(level)

	// Configure output
	output := cfg.Output
	if output == nil {
		output = os.Stdout
	}

	// Pretty print for development
	if cfg.Pretty {
		output = zerolog.ConsoleWriter{
			Out:        output,
			TimeFormat: time.RFC3339,
			NoColor:    false,
		}
	}

	// Configure global logger
	logger := zerolog.New(output).
		With().
		Timestamp().
		Str("service", "wotan").
		Logger()

	if cfg.WithCaller {
		logger = logger.With().Caller().Logger()
	}

	log.Logger = logger
}

// FromContext retrieves the logger from context
func FromContext(ctx context.Context) *zerolog.Logger {
	if logger, ok := ctx.Value(loggerKey).(*zerolog.Logger); ok {
		return logger
	}
	return &log.Logger
}

// WithContext adds logger to context
func WithContext(ctx context.Context, logger *zerolog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey, logger)
}

// WithRequestID adds request ID to logger and context
func WithRequestID(ctx context.Context, requestID string) (context.Context, *zerolog.Logger) {
	logger := FromContext(ctx).With().Str("request_id", requestID).Logger()
	return WithContext(ctx, &logger), &logger
}

// WithMember adds member ID to logger
func WithMember(logger *zerolog.Logger, memberID string) *zerolog.Logger {
	l := logger.With().Str("member_id", memberID).Logger()
	return &l
}

// WithRoom adds room ID to logger
func WithRoom(logger *zerolog.Logger, roomID string) *zerolog.Logger {
	l := logger.With().Str("room_id", roomID).Logger()
	return &l
}

// HTTPRequest logs HTTP request details
func HTTPRequest(logger *zerolog.Logger, method, path, remoteAddr string, statusCode int, duration time.Duration) {
	logger.Info().
		Str("method", method).
		Str("path", path).
		Str("remote_addr", remoteAddr).
		Int("status_code", statusCode).
		Dur("duration_ms", duration).
		Msg("http_request")
}

// Error logs error with context
func Error(ctx context.Context, err error, msg string) {
	FromContext(ctx).Error().
		Err(err).
		Msg(msg)
}

// Info logs info message
func Info(ctx context.Context, msg string) {
	FromContext(ctx).Info().Msg(msg)
}

// Debug logs debug message
func Debug(ctx context.Context, msg string) {
	FromContext(ctx).Debug().Msg(msg)
}

// Warn logs warning message
func Warn(ctx context.Context, msg string) {
	FromContext(ctx).Warn().Msg(msg)
}

// Fatal logs fatal message and exits
func Fatal(ctx context.Context, msg string) {
	FromContext(ctx).Fatal().Msg(msg)
}
