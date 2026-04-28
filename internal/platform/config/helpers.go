package config

import "strconv"

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
