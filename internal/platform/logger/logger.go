// Package logger provides helper functions to create configured zerolog loggers.
package logger

import (
	"os"
	"time"

	"github.com/rs/zerolog"
)

// NewPrettyWithLevel returns a console (human-readable) logger at the given level.
// Use in development and in the main API server.
func NewPrettyWithLevel(lvl zerolog.Level) zerolog.Logger {
	return zerolog.New(
		zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: time.RFC3339,
		},
	).With().
		Timestamp().
		Logger().
		Level(lvl)
}

// NewWithLevel returns a JSON logger at the given level.
// Use in background workers and production deployments.
func NewWithLevel(lvl zerolog.Level) zerolog.Logger {
	return zerolog.New(os.Stdout).
		With().
		Timestamp().
		Logger().
		Level(lvl)
}
