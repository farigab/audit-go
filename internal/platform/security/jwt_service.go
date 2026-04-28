// Package security provides JWT helper services used for authentication.
package security

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// TokenService defines the JWT operations required by the application.
type TokenService interface {
	GenerateToken(userLogin string, extraClaims map[string]interface{}) (string, error)
	ExtractUserLogin(tokenStr string) (string, error)
	IsValid(tokenStr string) bool
	IsExpired(tokenStr string) bool
}

// JWTService implements TokenService using HMAC SHA256.
type JWTService struct {
	secret                []byte
	accessTokenExpiration time.Duration
}

// NewJWTService creates a JWTService configured with the given secret and expiration.
// Returns an error when secret is empty so the caller (main) can handle it explicitly
// without the hidden os.Exit that prevented unit testing.
func NewJWTService(secret string, accessTokenExpirationSeconds int64) (*JWTService, error) {
	if secret == "" {
		return nil, errors.New("JWT_SECRET must not be empty")
	}
	return &JWTService{
		secret:                []byte(secret),
		accessTokenExpiration: time.Duration(accessTokenExpirationSeconds) * time.Second,
	}, nil
}

// GenerateToken builds and signs a JWT for the provided user login.
func (s *JWTService) GenerateToken(userLogin string, extraClaims map[string]interface{}) (string, error) {
	now := time.Now()
	claims := jwt.MapClaims{}
	claims["login"] = userLogin
	for k, v := range extraClaims {
		claims[k] = v
	}
	claims["iat"] = now.Unix()
	claims["exp"] = now.Add(s.accessTokenExpiration).Unix()

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.secret)
}

// parseToken parses the token string and returns claims if present.
// When the token parsed but validation failed (e.g. expired), both claims
// and a non-nil error may be returned — callers check accordingly.
func (s *JWTService) parseToken(tokenStr string) (jwt.MapClaims, error) {
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return s.secret, nil
	})

	if err != nil {
		if token != nil {
			if claims, ok := token.Claims.(jwt.MapClaims); ok {
				return claims, err
			}
		}
		return nil, err
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok {
		return claims, nil
	}
	return nil, errors.New("invalid token claims")
}

// ExtractUserLogin extracts the `login` claim from the token or returns an error.
func (s *JWTService) ExtractUserLogin(tokenStr string) (string, error) {
	claims, err := s.parseToken(tokenStr)
	if err != nil {
		return "", err
	}
	if login, ok := claims["login"].(string); ok {
		return login, nil
	}
	return "", errors.New("login claim missing")
}

// IsValid returns true when the token parses and validates correctly.
func (s *JWTService) IsValid(tokenStr string) bool {
	_, err := s.parseToken(tokenStr)
	return err == nil
}

// IsExpired returns true when token is expired or cannot be parsed.
func (s *JWTService) IsExpired(tokenStr string) bool {
	claims, _ := s.parseToken(tokenStr)
	if claims == nil {
		return true
	}
	if exp, ok := claims["exp"].(float64); ok {
		return time.Unix(int64(exp), 0).Before(time.Now())
	}
	return true
}

// ExtractUserLoginSafe returns the login claim even when the token failed
// validation (e.g. expired). Used in logout-like paths where we need the
// identity but do not require a valid signature.
func (s *JWTService) ExtractUserLoginSafe(tokenStr string) (string, error) {
	claims, err := s.parseToken(tokenStr)
	if claims == nil {
		return "", err
	}
	if login, ok := claims["login"].(string); ok && login != "" {
		return login, nil
	}
	if err != nil {
		return "", err
	}
	return "", errors.New("login claim missing")
}
