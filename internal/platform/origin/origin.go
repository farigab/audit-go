// Package origin provides utilities for validating HTTP Origin headers
// against a configured allowlist. Used by both the CORS middleware and
// the refresh-token rotation handler.
//
// Formato da allowlist: string separada por vírgulas com URLs completas
// ou hosts bare. Exemplos:
//
//	"https://app.example.com,https://admin.example.com"
//	"http://localhost:3000"
//	"app.example.com:443"
package origin

import (
	"net"
	"net/url"
	"strings"
)

// Allowlist holds a parsed set of allowed origin hostnames.
// Construída uma vez na inicialização e reutilizada em cada request —
// zero alocações no hot path.
type Allowlist struct {
	hostnames []string // hostname bare, sem porta e sem scheme
	raw       string   // valor original, para logging
}

// Parse builds an Allowlist from a comma-separated string of URLs or hosts.
// Entradas em branco são ignoradas. Retorna uma Allowlist vazia (que bloqueia
// tudo) se raw for vazio.
func Parse(raw string) Allowlist {
	if raw == "" {
		return Allowlist{}
	}

	seen := make(map[string]struct{})
	out := make([]string, 0)

	for _, entry := range strings.Split(raw, ",") {
		h := extractHostname(strings.TrimSpace(entry))
		if h == "" {
			continue
		}
		if _, dup := seen[h]; dup {
			continue
		}
		seen[h] = struct{}{}
		out = append(out, h)
	}

	return Allowlist{hostnames: out, raw: raw}
}

// Empty reports whether no origins are configured.
// Um allowlist vazia significa "negar tudo que tiver Origin header".
func (a Allowlist) Empty() bool {
	return len(a.hostnames) == 0
}

// Allows reports whether the given Origin header value is permitted.
//
// Regras:
//  1. Origin vazio (request não-browser, ex: curl, server-to-server) → sempre true.
//  2. Allowlist vazia → false para qualquer Origin presente.
//  3. Caso contrário, compara hostname do Origin contra a lista.
func (a Allowlist) Allows(originHeader string) bool {
	if originHeader == "" {
		return true // não-browser, sem restrição de origin
	}
	if a.Empty() {
		return false
	}
	h := hostnameFromOriginHeader(originHeader)
	if h == "" {
		return false
	}
	for _, allowed := range a.hostnames {
		if allowed == h {
			return true
		}
	}
	return false
}

// AllowedOriginFor retorna o valor exato que deve ir no header
// Access-Control-Allow-Origin para o request dado. Retorna "" se não permitido.
//
// Por que retornar a string do request e não "*"?
// Quando credenciais (cookies) estão envolvidas, o browser rejeita "*".
// Devemos ecoar o Origin exato do request.
func (a Allowlist) AllowedOriginFor(originHeader string) string {
	if !a.Allows(originHeader) {
		return ""
	}
	if originHeader == "" {
		return "*"
	}
	return originHeader
}

// Raw returns the original unparsed string (useful for logging/debugging).
func (a Allowlist) Raw() string { return a.raw }

// ── helpers ───────────────────────────────────────────────────────────────────

// hostnameFromOriginHeader extrai apenas o hostname de um header Origin completo.
// O header Origin tem formato: scheme://host[:port] — sem path, query ou fragment.
func hostnameFromOriginHeader(origin string) string {
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return ""
	}
	return u.Hostname() // strip porta se presente
}

// extractHostname normaliza uma entrada da allowlist para hostname bare.
// Aceita URLs completas ("https://example.com:8080") ou bare hosts ("example.com:8080").
func extractHostname(raw string) string {
	if raw == "" {
		return ""
	}
	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err != nil {
			return raw
		}
		return u.Hostname()
	}
	// bare host[:port]
	if h, _, err := net.SplitHostPort(raw); err == nil {
		return h
	}
	return raw
}
