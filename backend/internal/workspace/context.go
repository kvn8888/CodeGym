package workspace

import "context"

type contextKey struct{}

// Scope represents the resolved workspace context for a request.
type Scope struct {
	WorkspaceID string `json:"workspace_id"`
}

// WithScope stores workspace scope in context.
func WithScope(ctx context.Context, scope Scope) context.Context {
	return context.WithValue(ctx, contextKey{}, scope)
}

// ScopeFromContext retrieves workspace scope from context.
func ScopeFromContext(ctx context.Context) (Scope, bool) {
	scope, ok := ctx.Value(contextKey{}).(Scope)
	return scope, ok
}
