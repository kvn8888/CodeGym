package tenant

import "context"

type contextKey struct{}

type Scope struct {
	TenantID string `json:"tenant_id"`
}

func WithScope(ctx context.Context, scope Scope) context.Context {
	return context.WithValue(ctx, contextKey{}, scope)
}

func ScopeFromContext(ctx context.Context) (Scope, bool) {
	scope, ok := ctx.Value(contextKey{}).(Scope)
	return scope, ok
}
