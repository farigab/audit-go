package origin

import "testing"

func TestAllowlistMatchesFullOriginIncludingPort(t *testing.T) {
	al := Parse("http://localhost:3000")

	if !al.Allows("http://localhost:3000") {
		t.Fatal("expected configured origin to be allowed")
	}
	if al.Allows("http://localhost:5173") {
		t.Fatal("expected different port to be denied")
	}
	if al.Allows("https://localhost:3000") {
		t.Fatal("expected different scheme to be denied")
	}
}

func TestAllowlistLegacyHostMatchesExactHostPort(t *testing.T) {
	al := Parse("localhost:3000")

	if !al.Allows("http://localhost:3000") {
		t.Fatal("expected exact legacy host:port to be allowed")
	}
	if al.Allows("http://localhost:3001") {
		t.Fatal("expected different legacy port to be denied")
	}
}
