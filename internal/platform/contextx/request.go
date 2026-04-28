// Package contextx provides helpers for request-scoped context keys and values.
package contextx

import "context"

type key string

const (
	// RequestIDKey is the context key used to store the request id.
	RequestIDKey key = "request_id"
	// UserIDKey is the context key used to store the user id.
	UserIDKey key = "user_id"
	// TenantIDKey is the context key used to store the tenant id.
	TenantIDKey key = "tenant_id"
)

// Set returns a new context with value v stored under key k.
func Set(ctx context.Context, k key, v string) context.Context {
	return context.WithValue(ctx, k, v)
}

// Get returns the string value stored under key k, or empty string if not present.
func Get(ctx context.Context, k key) string {
	val := ctx.Value(k)

	s, ok := val.(string)
	if !ok {
		return ""
	}

	return s
}
