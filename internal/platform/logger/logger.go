package logger

import (
	"os"
	"time"

	"github.com/rs/zerolog"
)

func level() zerolog.Level {
	return zerolog.InfoLevel
}

func New() zerolog.Logger {
	return zerolog.New(os.Stdout).
		With().
		Timestamp().
		Logger().
		Level(level())
}

func NewPretty() zerolog.Logger {
	return NewPrettyWithLevel(level())
}

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

func NewWithLevel(lvl zerolog.Level) zerolog.Logger {
	return zerolog.New(os.Stdout).
		With().
		Timestamp().
		Logger().
		Level(lvl)
}
