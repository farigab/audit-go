package domain

import "time"

// RefreshToken represents a stored refresh token linked to a user.
type RefreshToken struct {
	Token     string    `db:"token" json:"token"`
	UserLogin string    `db:"user_login" json:"userLogin"`
	ExpiresAt time.Time `db:"expires_at" json:"expiresAt"`
	CreatedAt time.Time `db:"created_at" json:"createdAt"`
	Revoked   bool      `db:"revoked" json:"revoked"`
}

// NewRefreshToken constructs a new RefreshToken with now as CreatedAt.
func NewRefreshToken(token, userLogin string, expiresAt time.Time) *RefreshToken {
	return &RefreshToken{Token: token, UserLogin: userLogin, ExpiresAt: expiresAt, CreatedAt: time.Now(), Revoked: false}
}
