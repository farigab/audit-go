// Package security provides JWT validation for Microsoft Entra ID (Azure AD) tokens.
package security

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// EntraConfig holds the required Microsoft Entra ID configuration.
type EntraConfig struct {
	TenantID string
	ClientID string
}

type entraClaims struct {
	jwt.RegisteredClaims
	OID               string   `json:"oid"`
	PreferredUsername string   `json:"preferred_username"`
	Name              string   `json:"name"`
	Nonce             string   `json:"nonce"`
	Roles             []string `json:"roles"`
}

type jwksKey struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	N   string `json:"n"`
	E   string `json:"e"`
}

type jwksResponse struct {
	Keys []jwksKey `json:"keys"`
}

// EntraTokenValidator validates Microsoft Entra ID access tokens.
type EntraTokenValidator struct {
	cfg          EntraConfig
	validIssuers map[string]struct{} // pre-computed at construction time
	httpClient   *http.Client

	mu        sync.RWMutex
	keyCache  map[string]*rsa.PublicKey
	cacheTime time.Time
	cacheTTL  time.Duration
}

// NewEntraTokenValidator creates a validator for the given Entra tenant and app.
func NewEntraTokenValidator(cfg EntraConfig) (*EntraTokenValidator, error) {
	if cfg.TenantID == "" {
		return nil, errors.New("entra: TenantID is required")
	}
	if cfg.ClientID == "" {
		return nil, errors.New("entra: ClientID is required")
	}

	return &EntraTokenValidator{
		cfg: cfg,
		// Both issuer formats are used depending on token version and tenant type.
		validIssuers: map[string]struct{}{
			fmt.Sprintf("https://login.microsoftonline.com/%s/v2.0", cfg.TenantID): {},
			fmt.Sprintf("https://sts.windows.net/%s/", cfg.TenantID):               {},
		},
		httpClient: &http.Client{Timeout: 10 * time.Second},
		keyCache:   make(map[string]*rsa.PublicKey),
		cacheTTL:   1 * time.Hour,
	}, nil
}

// EntraClaims holds the validated claims extracted from an Entra ID token.
type EntraClaims struct {
	OID   string
	Login string // preferred_username (UPN), falls back to OID for service principals
	Name  string
	Nonce string
	Roles []string
}

// Validate parses and validates a raw Entra ID bearer token.
func (v *EntraTokenValidator) Validate(ctx context.Context, rawToken string) (*EntraClaims, error) {
	// Parse without verifying to grab the kid from the header.
	unverified, _, err := new(jwt.Parser).ParseUnverified(rawToken, &entraClaims{})
	if err != nil {
		return nil, fmt.Errorf("entra: parsing token header: %w", err)
	}

	kid, ok := unverified.Header["kid"].(string)
	if !ok || kid == "" {
		return nil, errors.New("entra: missing kid in token header")
	}

	pubKey, err := v.getKey(ctx, kid)
	if err != nil {
		return nil, fmt.Errorf("entra: resolving signing key: %w", err)
	}

	claims := &entraClaims{}
	token, err := jwt.ParseWithClaims(
		rawToken,
		claims,
		func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
			}
			return pubKey, nil
		},
		jwt.WithAudience(v.cfg.ClientID),
		jwt.WithExpirationRequired(),
		// Issuer validated manually below — jwt.WithIssuer accepts only one value.
	)
	if err != nil {
		return nil, fmt.Errorf("entra: token validation failed: %w", err)
	}
	if !token.Valid {
		return nil, errors.New("entra: token is not valid")
	}

	// Multi-issuer check: Entra can use two different issuer URL formats.
	iss, err := claims.GetIssuer()
	if err != nil || iss == "" {
		return nil, errors.New("entra: missing issuer claim")
	}
	if _, trusted := v.validIssuers[iss]; !trusted {
		return nil, fmt.Errorf("entra: untrusted issuer %q", iss)
	}

	login := claims.PreferredUsername
	if login == "" {
		login = claims.OID // fallback for service principals
	}

	return &EntraClaims{
		OID:   claims.OID,
		Login: login,
		Name:  claims.Name,
		Nonce: claims.Nonce,
		Roles: claims.Roles,
	}, nil
}

// getKey returns the RSA public key for the given kid, using the cache.
func (v *EntraTokenValidator) getKey(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	v.mu.RLock()
	key, found := v.keyCache[kid]
	expired := time.Since(v.cacheTime) > v.cacheTTL
	v.mu.RUnlock()

	if found && !expired {
		return key, nil
	}

	return v.refreshKeys(ctx, kid)
}

// refreshKeys fetches the JWKS from Microsoft and updates the cache.
func (v *EntraTokenValidator) refreshKeys(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	jwksURL := fmt.Sprintf(
		"https://login.microsoftonline.com/%s/discovery/v2.0/keys",
		v.cfg.TenantID,
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, jwksURL, nil)
	if err != nil {
		return nil, fmt.Errorf("building JWKS request: %w", err)
	}

	resp, err := v.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching JWKS: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("JWKS endpoint returned %d", resp.StatusCode)
	}

	var jwks jwksResponse
	if err = json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		return nil, fmt.Errorf("decoding JWKS: %w", err)
	}

	newCache := make(map[string]*rsa.PublicKey, len(jwks.Keys))
	for _, k := range jwks.Keys {
		if k.Kty != "RSA" {
			continue
		}
		pub, err := rsaPublicKeyFromJWK(k)
		if err != nil {
			continue // skip malformed keys; log in production
		}
		newCache[k.Kid] = pub
	}

	v.mu.Lock()
	v.keyCache = newCache
	v.cacheTime = time.Now()
	v.mu.Unlock()

	key, ok := newCache[kid]
	if !ok {
		return nil, fmt.Errorf("signing key %q not found in JWKS", kid)
	}

	return key, nil
}

// rsaPublicKeyFromJWK reconstructs an *rsa.PublicKey from a JWK entry.
func rsaPublicKeyFromJWK(k jwksKey) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(k.N)
	if err != nil {
		return nil, fmt.Errorf("decoding modulus: %w", err)
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(k.E)
	if err != nil {
		return nil, fmt.Errorf("decoding exponent: %w", err)
	}

	return &rsa.PublicKey{
		N: new(big.Int).SetBytes(nBytes),
		E: int(new(big.Int).SetBytes(eBytes).Int64()),
	}, nil
}
