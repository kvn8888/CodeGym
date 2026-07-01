package tenant

import "context"

type contextKey struct{}

// Scope represents the resolved tenant context for a request.
type Scope struct {
	TenantID string `json:"tenant_id"`
}

// WithScope stores tenant scope in context.
func WithScope(ctx context.Context, scope Scope) context.Context {
	return context.WithValue(ctx, contextKey{}, scope)
}

// ScopeFromContext retrieves tenant scope from context.
func ScopeFromContext(ctx context.Context) (Scope, bool) {
	scope, ok := ctx.Value(contextKey{}).(Scope)
	return scope, ok
}
