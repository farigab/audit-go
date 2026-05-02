// Package origin provides utilities for validating HTTP Origin headers
// against a configured allowlist.
//
// Allowlist format: comma-separated origins. Full origins are preferred:
//
//	"https://app.example.com,https://admin.example.com"
//	"http://localhost:3000"
//
// Bare host entries are accepted for local/dev compatibility and match the
// exact host[:port] from the Origin header.
package origin

import (
	"net"
	"net/url"
	"strings"
)

// Allowlist holds a parsed set of allowed origins.
type Allowlist struct {
	origins     []string
	legacyHosts []string
}

// Parse builds an Allowlist from a comma-separated string of origins.
func Parse(raw string) Allowlist {
	if raw == "" {
		return Allowlist{}
	}

	seen := make(map[string]struct{})
	origins := make([]string, 0)
	legacyHosts := make([]string, 0)

	for _, entry := range strings.Split(raw, ",") {
		value := strings.TrimSpace(entry)
		if value == "" {
			continue
		}

		origin, legacyHost := normalizeAllowlistEntry(value)
		key := origin
		if key == "" {
			key = "host:" + legacyHost
		}
		if key == "" {
			continue
		}
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}

		if origin != "" {
			origins = append(origins, origin)
			continue
		}
		legacyHosts = append(legacyHosts, legacyHost)
	}

	return Allowlist{origins: origins, legacyHosts: legacyHosts}
}

// Empty reports whether no origins are configured.
func (a Allowlist) Empty() bool {
	return len(a.origins) == 0 && len(a.legacyHosts) == 0
}

// Allows reports whether the given Origin header value is permitted.
func (a Allowlist) Allows(originHeader string) bool {
	if originHeader == "" {
		return true
	}
	if a.Empty() {
		return false
	}

	origin, host := normalizeOriginHeader(originHeader)
	if origin == "" {
		return false
	}
	for _, allowed := range a.origins {
		if allowed == origin {
			return true
		}
	}
	for _, allowed := range a.legacyHosts {
		if allowed == host {
			return true
		}
	}
	return false
}

func normalizeAllowlistEntry(raw string) (origin string, legacyHost string) {
	if raw == "" {
		return "", ""
	}
	if strings.Contains(raw, "://") {
		origin, _ := normalizeOriginHeader(raw)
		return origin, ""
	}
	return "", normalizeHost(raw)
}

func normalizeOriginHeader(raw string) (origin string, host string) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", ""
	}
	if u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return "", ""
	}

	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", ""
	}

	host = normalizeHost(u.Host)
	if host == "" {
		return "", ""
	}

	return scheme + "://" + host, host
}

func normalizeHost(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "" {
		return ""
	}

	host, port, err := net.SplitHostPort(raw)
	if err == nil {
		return strings.ToLower(net.JoinHostPort(host, port))
	}

	return raw
}
