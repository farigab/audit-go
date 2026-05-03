// Package app implements access and authentication application services.
package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"audit-go/internal/features/access"
	"audit-go/internal/platform/security"
)

const (
	defaultOAuthStateTTL = 10 * time.Minute
)

var (
	ErrInvalidCallback = errors.New("access auth: invalid callback")
	ErrInvalidRefresh  = errors.New("access auth: invalid refresh token")
	ErrRefreshReuse    = errors.New("access auth: refresh token reuse detected")
)

// ClientMetadata carries request-derived session metadata.
type ClientMetadata struct {
	IPAddress string
	UserAgent string
}

// AuthState is a stored OAuth authorization attempt.
type AuthState struct {
	StateHash    string
	CodeVerifier string
	Nonce        string
	ReturnURL    string
	ExpiresAt    time.Time
}

// SessionRecord is a stored application session.
type SessionRecord struct {
	TokenHash  string
	SessionID  string
	UserLogin  string
	IPAddress  string
	UserAgent  string
	LastSeenAt time.Time
	ExpiresAt  time.Time
}

// RefreshTokenRecord is a stored rotating application refresh token.
type RefreshTokenRecord struct {
	TokenHash string
	SessionID string
	UserLogin string
	ExpiresAt time.Time
}

// RefreshTokenIdentity links a refresh token to its user and session family.
type RefreshTokenIdentity struct {
	UserLogin string
	SessionID string
}

// UserProfile is the application user projection maintained from Entra claims.
type UserProfile struct {
	Login    string
	EntraOID string
	Name     string
}

type store interface {
	CreateAuthState(ctx context.Context, state AuthState) error
	ConsumeAuthState(ctx context.Context, stateHash string, now time.Time) (*AuthState, error)
	UpsertUser(ctx context.Context, user UserProfile) error
	PrincipalByLogin(ctx context.Context, login string) (access.Principal, error)
	CreateSession(ctx context.Context, record SessionRecord) error
	PrincipalBySession(ctx context.Context, tokenHash string, now time.Time) (access.Principal, error)
	RevokeSession(ctx context.Context, tokenHash string) error
	CreateRefreshToken(ctx context.Context, record RefreshTokenRecord) error
	RotateRefreshToken(ctx context.Context, oldHash string, next RefreshTokenRecord, now time.Time) (RefreshTokenIdentity, error)
	PrincipalByRefreshToken(ctx context.Context, tokenHash string, now time.Time) (access.Principal, error)
	RevokeRefreshToken(ctx context.Context, tokenHash string) error
}

// Config contains settings for the BFF authentication flow.
type Config struct {
	TenantID           string
	ClientID           string
	ClientSecret       string
	RedirectURL        string
	SuccessRedirectURL string
	AllowedOrigins     string
	SessionTTL         time.Duration
	RefreshTTL         time.Duration
}

// Service handles Microsoft Entra login and application session lifecycle.
type Service struct {
	cfg       Config
	store     store
	validator *security.EntraTokenValidator
	client    *http.Client
}

// AuthResult contains newly issued application tokens.
type AuthResult struct {
	Principal        access.Principal
	SessionToken     string
	SessionExpiresAt time.Time
	RefreshToken     string
	RefreshExpiresAt time.Time
	CSRFToken        string
	ReturnURL        string
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	IDToken     string `json:"id_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

// NewService creates an authentication service.
func NewService(cfg Config, store store, validator *security.EntraTokenValidator) *Service {
	if cfg.SessionTTL == 0 {
		cfg.SessionTTL = 15 * time.Minute
	}
	if cfg.RefreshTTL == 0 {
		cfg.RefreshTTL = 30 * 24 * time.Hour
	}

	return &Service{
		cfg:       cfg,
		store:     store,
		validator: validator,
		client:    &http.Client{Timeout: 10 * time.Second},
	}
}

// LoginURL starts the Entra authorization code + PKCE flow.
func (s *Service) LoginURL(ctx context.Context, returnURL string) (string, error) {
	state, err := randomToken(32)
	if err != nil {
		return "", err
	}
	verifier, err := randomToken(32)
	if err != nil {
		return "", err
	}
	nonce, err := randomToken(32)
	if err != nil {
		return "", err
	}

	stateHash := tokenHash(state)
	if err = s.store.CreateAuthState(ctx, AuthState{
		StateHash:    stateHash,
		CodeVerifier: verifier,
		Nonce:        nonce,
		ReturnURL:    s.safeReturnURL(returnURL),
		ExpiresAt:    time.Now().UTC().Add(defaultOAuthStateTTL),
	}); err != nil {
		return "", fmt.Errorf("creating auth state: %w", err)
	}

	u, err := url.Parse(s.authorizeURL())
	if err != nil {
		return "", fmt.Errorf("building authorization URL: %w", err)
	}

	q := u.Query()
	q.Set("client_id", s.cfg.ClientID)
	q.Set("response_type", "code")
	q.Set("redirect_uri", s.cfg.RedirectURL)
	q.Set("response_mode", "query")
	q.Set("scope", "openid profile email")
	q.Set("state", state)
	q.Set("nonce", nonce)
	q.Set("code_challenge", codeChallenge(verifier))
	q.Set("code_challenge_method", "S256")
	u.RawQuery = q.Encode()

	return u.String(), nil
}

// CompleteCallback exchanges the authorization code and issues app cookies.
func (s *Service) CompleteCallback(ctx context.Context, code, state string, client ClientMetadata) (*AuthResult, error) {
	if code == "" || state == "" {
		return nil, ErrInvalidCallback
	}

	authState, err := s.store.ConsumeAuthState(ctx, tokenHash(state), time.Now().UTC())
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidCallback, err)
	}

	token, err := s.exchangeCode(ctx, code, authState.CodeVerifier)
	if err != nil {
		return nil, err
	}
	if token.IDToken == "" {
		return nil, errors.New("entra token response did not include id_token")
	}

	claims, err := s.validator.Validate(ctx, token.IDToken)
	if err != nil {
		return nil, fmt.Errorf("validating id_token: %w", err)
	}
	if claims.Nonce != authState.Nonce {
		return nil, fmt.Errorf("%w: nonce mismatch", ErrInvalidCallback)
	}

	login := claims.Login
	if login == "" {
		login = claims.OID
	}
	name := claims.Name
	if name == "" {
		name = login
	}

	if err = s.store.UpsertUser(ctx, UserProfile{
		Login:    login,
		EntraOID: claims.OID,
		Name:     name,
	}); err != nil {
		return nil, fmt.Errorf("upserting user: %w", err)
	}

	principal, err := s.store.PrincipalByLogin(ctx, login)
	if err != nil {
		return nil, fmt.Errorf("loading principal: %w", err)
	}
	if claims.OID != "" {
		principal.ID = claims.OID
	}
	if principal.Name == "" {
		principal.Name = name
	}

	result, err := s.issueTokens(ctx, principal, client)
	if err != nil {
		return nil, err
	}
	result.ReturnURL = authState.ReturnURL

	return result, nil
}

// Refresh rotates the application refresh token, revokes the current session,
// and issues a replacement session.
func (s *Service) Refresh(
	ctx context.Context,
	refreshToken, currentSessionToken string,
	client ClientMetadata,
) (*AuthResult, error) {
	if refreshToken == "" {
		return nil, ErrInvalidRefresh
	}

	nextRefresh, err := randomToken(32)
	if err != nil {
		return nil, err
	}
	nextRefreshExpires := time.Now().UTC().Add(s.cfg.RefreshTTL)
	oldHash := tokenHash(refreshToken)

	principal, sessionID, err := s.rotateRefresh(ctx, oldHash, nextRefresh, nextRefreshExpires)
	if err != nil {
		return nil, err
	}
	if currentSessionToken != "" {
		if err = s.store.RevokeSession(ctx, tokenHash(currentSessionToken)); err != nil {
			return nil, fmt.Errorf("revoking current session: %w", err)
		}
	}

	sessionToken, sessionExpires, csrfToken, err := s.createSession(ctx, principal, sessionID, client)
	if err != nil {
		return nil, err
	}

	return &AuthResult{
		Principal:        principal,
		SessionToken:     sessionToken,
		SessionExpiresAt: sessionExpires,
		RefreshToken:     nextRefresh,
		RefreshExpiresAt: nextRefreshExpires,
		CSRFToken:        csrfToken,
		ReturnURL:        s.cfg.SuccessRedirectURL,
	}, nil
}

// PrincipalFromSession resolves an opaque app session token.
func (s *Service) PrincipalFromSession(ctx context.Context, sessionToken string) (access.Principal, error) {
	if sessionToken == "" {
		return access.Principal{}, access.ErrUnauthenticated
	}
	principal, err := s.store.PrincipalBySession(ctx, tokenHash(sessionToken), time.Now().UTC())
	if err != nil {
		return access.Principal{}, access.ErrUnauthenticated
	}
	return principal, nil
}

// Logout revokes the current app session and refresh token when present.
func (s *Service) Logout(ctx context.Context, sessionToken, refreshToken string) error {
	if sessionToken != "" {
		if err := s.store.RevokeSession(ctx, tokenHash(sessionToken)); err != nil {
			return fmt.Errorf("revoking session: %w", err)
		}
	}
	if refreshToken != "" {
		if err := s.store.RevokeRefreshToken(ctx, tokenHash(refreshToken)); err != nil {
			return fmt.Errorf("revoking refresh token: %w", err)
		}
	}
	return nil
}

func (s *Service) issueTokens(
	ctx context.Context,
	principal access.Principal,
	client ClientMetadata,
) (*AuthResult, error) {
	sessionID := uuid.NewString()
	sessionToken, sessionExpires, csrfToken, err := s.createSession(ctx, principal, sessionID, client)
	if err != nil {
		return nil, err
	}

	refreshToken, err := randomToken(32)
	if err != nil {
		return nil, err
	}
	refreshExpires := time.Now().UTC().Add(s.cfg.RefreshTTL)
	if err = s.store.CreateRefreshToken(ctx, RefreshTokenRecord{
		TokenHash: tokenHash(refreshToken),
		SessionID: sessionID,
		UserLogin: principal.UserKey(),
		ExpiresAt: refreshExpires,
	}); err != nil {
		return nil, fmt.Errorf("creating refresh token: %w", err)
	}

	return &AuthResult{
		Principal:        principal,
		SessionToken:     sessionToken,
		SessionExpiresAt: sessionExpires,
		RefreshToken:     refreshToken,
		RefreshExpiresAt: refreshExpires,
		CSRFToken:        csrfToken,
		ReturnURL:        s.cfg.SuccessRedirectURL,
	}, nil
}

func (s *Service) createSession(
	ctx context.Context,
	principal access.Principal,
	sessionID string,
	client ClientMetadata,
) (sessionToken string, expiresAt time.Time, csrfToken string, err error) {
	sessionToken, err = randomToken(32)
	if err != nil {
		return "", time.Time{}, "", err
	}
	csrfToken, err = randomToken(32)
	if err != nil {
		return "", time.Time{}, "", err
	}
	expiresAt = time.Now().UTC().Add(s.cfg.SessionTTL)
	lastSeenAt := time.Now().UTC()

	if err = s.store.CreateSession(ctx, SessionRecord{
		TokenHash:  tokenHash(sessionToken),
		SessionID:  sessionID,
		UserLogin:  principal.UserKey(),
		IPAddress:  client.IPAddress,
		UserAgent:  client.UserAgent,
		LastSeenAt: lastSeenAt,
		ExpiresAt:  expiresAt,
	}); err != nil {
		return "", time.Time{}, "", fmt.Errorf("creating session: %w", err)
	}

	return sessionToken, expiresAt, csrfToken, nil
}

func (s *Service) rotateRefresh(
	ctx context.Context,
	oldHash string,
	nextRefresh string,
	nextExpires time.Time,
) (access.Principal, string, error) {
	nextRecord := RefreshTokenRecord{
		TokenHash: tokenHash(nextRefresh),
		SessionID: uuid.NewString(),
		ExpiresAt: nextExpires,
	}

	// The store validates that oldHash exists, is not revoked, and has not expired.
	identity, err := s.store.RotateRefreshToken(ctx, oldHash, nextRecord, time.Now().UTC())
	if err != nil {
		if errors.Is(err, ErrRefreshReuse) {
			return access.Principal{}, "", fmt.Errorf("%w: %v", ErrRefreshReuse, err)
		}
		return access.Principal{}, "", fmt.Errorf("%w: %v", ErrInvalidRefresh, err)
	}
	if identity.SessionID == "" {
		identity.SessionID = nextRecord.SessionID
	}

	record, err := s.store.PrincipalByLogin(ctx, identity.UserLogin)
	if err != nil {
		return access.Principal{}, "", fmt.Errorf("loading principal: %w", err)
	}

	return record, identity.SessionID, nil
}

func (s *Service) exchangeCode(ctx context.Context, code, verifier string) (*tokenResponse, error) {
	form := url.Values{}
	form.Set("client_id", s.cfg.ClientID)
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", s.cfg.RedirectURL)
	form.Set("code_verifier", verifier)
	form.Set("scope", "openid profile email")
	if s.cfg.ClientSecret != "" {
		form.Set("client_secret", s.cfg.ClientSecret)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.tokenURL(), strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("building token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("exchanging code with Entra: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading Entra token response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("Entra token endpoint returned %d: %s", resp.StatusCode, body)
	}

	var out tokenResponse
	if err = json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("decoding Entra token response: %w", err)
	}
	return &out, nil
}

func (s *Service) safeReturnURL(raw string) string {
	if raw == "" {
		return s.cfg.SuccessRedirectURL
	}
	if strings.HasPrefix(raw, "/") && !strings.HasPrefix(raw, "//") {
		return raw
	}

	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return s.cfg.SuccessRedirectURL
	}

	requestOrigin := strings.ToLower(u.Scheme + "://" + u.Host)
	for _, entry := range strings.Split(s.cfg.AllowedOrigins, ",") {
		allowed := strings.TrimSpace(strings.ToLower(entry))
		if allowed != "" && allowed == requestOrigin {
			return raw
		}
	}

	return s.cfg.SuccessRedirectURL
}

func (s *Service) authorizeURL() string {
	return fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/authorize", s.cfg.TenantID)
}

func (s *Service) tokenURL() string {
	return fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/token", s.cfg.TenantID)
}

func codeChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func randomToken(size int) (string, error) {
	b := make([]byte, size)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating random token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func tokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
