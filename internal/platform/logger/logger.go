// Package logger provides helper functions to create configured zerolog loggers.
package logger

import (
	"os"
	"time"

	"github.com/rs/zerolog"
)

func level() zerolog.Level {
	return zerolog.InfoLevel
}

// New returns a default zerolog logger.
func New() zerolog.Logger {
	return zerolog.New(os.Stdout).
		With().
		Timestamp().
		Logger().
		Level(level())
}

// NewPretty returns a pretty (console) logger with the default level.
func NewPretty() zerolog.Logger {
	return NewPrettyWithLevel(level())
}

// NewPrettyWithLevel returns a pretty (console) logger with the given level.
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

// NewWithLevel returns a JSON logger with the given level.
func NewWithLevel(lvl zerolog.Level) zerolog.Logger {
	return zerolog.New(os.Stdout).
		With().
		Timestamp().
		Logger().
		Level(lvl)
}
