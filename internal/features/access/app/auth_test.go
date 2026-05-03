package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"audit-go/internal/features/access"
)

func TestRefreshRevokesCurrentSessionAndReusesSessionFamily(t *testing.T) {
	store := &fakeAuthStore{
		principal: access.Principal{Login: "auditor@example.com", Name: "Auditor"},
		rotateIdentity: RefreshTokenIdentity{
			UserLogin: "auditor@example.com",
			SessionID: "session-family-1",
		},
	}
	service := NewService(Config{SessionTTL: time.Minute, RefreshTTL: time.Hour}, store, nil)

	result, err := service.Refresh(context.Background(), "old-refresh", "old-session", ClientMetadata{
		IPAddress: "10.0.0.1",
		UserAgent: "unit-test",
	})
	if err != nil {
		t.Fatalf("expected refresh to succeed, got %v", err)
	}
	if result.SessionToken == "" || result.RefreshToken == "" || result.CSRFToken == "" {
		t.Fatalf("expected rotated auth tokens, got %#v", result)
	}
	if len(store.revokedSessions) != 1 {
		t.Fatalf("expected current session revoked once, got %d", len(store.revokedSessions))
	}
	if store.revokedSessions[0] == hashTokenForTest("old-refresh") {
		t.Fatalf("expected session revocation to use current session token hash")
	}
	if len(store.createdSessions) != 1 {
		t.Fatalf("expected one new session, got %d", len(store.createdSessions))
	}
	if got := store.createdSessions[0].SessionID; got != "session-family-1" {
		t.Fatalf("expected session family id reused, got %q", got)
	}
	if got := store.createdSessions[0].IPAddress; got != "10.0.0.1" {
		t.Fatalf("expected IP captured, got %q", got)
	}
	if got := store.createdSessions[0].UserAgent; got != "unit-test" {
		t.Fatalf("expected user agent captured, got %q", got)
	}
	if len(store.createdRefreshTokens) != 0 {
		t.Fatalf("expected refresh rotation, not a brand new refresh token insert")
	}
	if store.rotatedNext.SessionID != "session-family-1" {
		t.Fatalf("expected rotated refresh token to keep session family, got %q", store.rotatedNext.SessionID)
	}
}

func TestRefreshRejectsReuseDetection(t *testing.T) {
	store := &fakeAuthStore{rotateErr: ErrRefreshReuse}
	service := NewService(Config{SessionTTL: time.Minute, RefreshTTL: time.Hour}, store, nil)

	_, err := service.Refresh(context.Background(), "old-refresh", "old-session", ClientMetadata{})
	if !errors.Is(err, ErrRefreshReuse) {
		t.Fatalf("expected ErrRefreshReuse, got %v", err)
	}
	if len(store.revokedSessions) != 0 {
		t.Fatalf("expected no current-session revoke when rotation fails, got %d", len(store.revokedSessions))
	}
}

type fakeAuthStore struct {
	principal            access.Principal
	rotateIdentity       RefreshTokenIdentity
	rotateErr            error
	createdSessions      []SessionRecord
	createdRefreshTokens []RefreshTokenRecord
	revokedSessions      []string
	rotatedNext          RefreshTokenRecord
}

func (f *fakeAuthStore) CreateAuthState(context.Context, AuthState) error {
	return nil
}

func (f *fakeAuthStore) ConsumeAuthState(context.Context, string, time.Time) (*AuthState, error) {
	return nil, nil
}

func (f *fakeAuthStore) UpsertUser(context.Context, UserProfile) error {
	return nil
}

func (f *fakeAuthStore) PrincipalByLogin(context.Context, string) (access.Principal, error) {
	return f.principal, nil
}

func (f *fakeAuthStore) CreateSession(_ context.Context, record SessionRecord) error {
	f.createdSessions = append(f.createdSessions, record)
	return nil
}

func (f *fakeAuthStore) PrincipalBySession(context.Context, string, time.Time) (access.Principal, error) {
	return access.Principal{}, access.ErrUnauthenticated
}

func (f *fakeAuthStore) RevokeSession(_ context.Context, tokenHash string) error {
	f.revokedSessions = append(f.revokedSessions, tokenHash)
	return nil
}

func (f *fakeAuthStore) CreateRefreshToken(_ context.Context, record RefreshTokenRecord) error {
	f.createdRefreshTokens = append(f.createdRefreshTokens, record)
	return nil
}

func (f *fakeAuthStore) RotateRefreshToken(
	_ context.Context,
	_ string,
	next RefreshTokenRecord,
	_ time.Time,
) (RefreshTokenIdentity, error) {
	f.rotatedNext = next
	if f.rotateErr != nil {
		return RefreshTokenIdentity{}, f.rotateErr
	}
	next.SessionID = f.rotateIdentity.SessionID
	f.rotatedNext = next
	return f.rotateIdentity, nil
}

func (f *fakeAuthStore) PrincipalByRefreshToken(context.Context, string, time.Time) (access.Principal, error) {
	return access.Principal{}, access.ErrUnauthenticated
}

func (f *fakeAuthStore) RevokeRefreshToken(context.Context, string) error {
	return nil
}

func hashTokenForTest(token string) string {
	return tokenHash(token)
}
