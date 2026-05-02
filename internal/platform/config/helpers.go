package config

import (
	"strconv"
	"time"
)

func defaultString(v, d string) string {
	if v == "" {
		return d
	}
	return v
}

func defaultBool(v string, d bool) bool {
	if v == "" {
		return d
	}
	parsed, err := strconv.ParseBool(v)
	if err != nil {
		return d
	}
	return parsed
}

func defaultDuration(v string, d time.Duration) time.Duration {
	if v == "" {
		return d
	}
	parsed, err := time.ParseDuration(v)
	if err != nil {
		return d
	}
	return parsed
}
