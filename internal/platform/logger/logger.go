package logger

import (
	"os"
	"time"

	"github.com/rs/zerolog"
)

func level() zerolog.Level {
	switch os.Getenv("LOG_LEVEL") {
	case "debug":
		return zerolog.DebugLevel
	case "warn":
		return zerolog.WarnLevel
	case "error":
		return zerolog.ErrorLevel
	default:
		return zerolog.InfoLevel
	}
}

func New() zerolog.Logger {
	return zerolog.New(os.Stdout).
		With().
		Timestamp().
		Logger().
		Level(level())
}

func NewPretty() zerolog.Logger {
	return zerolog.New(
		zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: time.RFC3339,
		},
	).With().
		Timestamp().
		Logger().
		Level(level())
}
