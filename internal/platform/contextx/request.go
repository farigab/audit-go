package contextx

import "context"

type key string

const (
	RequestIDKey key = "request_id"
	UserIDKey    key = "user_id"
	TenantIDKey  key = "tenant_id"
)

func Set(ctx context.Context, k key, v string) context.Context {
	return context.WithValue(ctx, k, v)
}

func Get(ctx context.Context, k key) string {
	val := ctx.Value(k)

	s, ok := val.(string)
	if !ok {
		return ""
	}

	return s
}
