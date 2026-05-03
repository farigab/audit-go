package http

import (
	"net/http"
	"testing"
)

func TestClientIPIgnoresForwardedHeadersWithoutTrustProxy(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "/", nil)
	if err != nil {
		t.Fatalf("expected request creation to succeed: %v", err)
	}
	req.RemoteAddr = "10.0.0.5:1234"
	req.Header.Set("X-Forwarded-For", "198.51.100.7")
	req.Header.Set("X-Real-IP", "203.0.113.11")

	if got := clientIP(req, false); got != "10.0.0.5" {
		t.Fatalf("expected remote addr host, got %q", got)
	}
}

func TestClientIPUsesForwardedHeadersWhenTrustProxyEnabled(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "/", nil)
	if err != nil {
		t.Fatalf("expected request creation to succeed: %v", err)
	}
	req.RemoteAddr = "10.0.0.5:1234"
	req.Header.Set("X-Forwarded-For", "198.51.100.7, 203.0.113.11")

	if got := clientIP(req, true); got != "198.51.100.7" {
		t.Fatalf("expected forwarded client IP, got %q", got)
	}
}
