package middleware

import "context"

type contextKey string

const UserLoginKey contextKey = "userLogin"

// UserLoginFromContext extracts the authenticated user login from context.
func UserLoginFromContext(ctx context.Context) (string, bool) {
	login, ok := ctx.Value(UserLoginKey).(string)
	return login, ok && login != ""
}
