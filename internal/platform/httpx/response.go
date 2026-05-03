// Package httpx contains small shared HTTP helpers.
package httpx

import (
	"encoding/json"
	"net/http"
)

// WriteText writes plain text with the given status code.
func WriteText(w http.ResponseWriter, status int, text string) error {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)

	_, err := w.Write([]byte(text))
	return err
}

// WriteJSON writes a JSON response with the given status code.
func WriteJSON(w http.ResponseWriter, status int, data any) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)

	return json.NewEncoder(w).Encode(data)
}

// WriteError writes a simple JSON error response.
func WriteError(w http.ResponseWriter, status int, message string) error {
	return WriteJSON(w, status, map[string]string{
		"error": message,
	})
}
