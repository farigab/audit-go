package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResponseWriterKeepsFirstStatusCode(t *testing.T) {
	recorder := httptest.NewRecorder()
	rw := newResponseWriter(recorder)

	rw.WriteHeader(http.StatusCreated)
	rw.WriteHeader(http.StatusInternalServerError)

	if got := rw.status; got != http.StatusCreated {
		t.Fatalf("expected first status to win, got %d", got)
	}
	if got := recorder.Result().StatusCode; got != http.StatusCreated {
		t.Fatalf("expected underlying response to keep first status, got %d", got)
	}
}

func TestResponseWriterImplicitlyTracksStatusOnWrite(t *testing.T) {
	recorder := httptest.NewRecorder()
	rw := newResponseWriter(recorder)

	if _, err := rw.Write([]byte("ok")); err != nil {
		t.Fatalf("expected write to succeed: %v", err)
	}

	if got := rw.status; got != http.StatusOK {
		t.Fatalf("expected implicit 200 status, got %d", got)
	}
	if got := rw.bytes; got != 2 {
		t.Fatalf("expected bytes to be tracked, got %d", got)
	}
}
