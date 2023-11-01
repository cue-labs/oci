package ociauth

import (
	"context"
)

type scopeKey struct{}

// ContextWithScope returns context annotated with the given
// scope. When ociclient receives a request with a scope in the context,
// it will treat it as "desired authorization scope"; new authorization tokens
// will be acquired with that scope as well as any scope required by
// the operation.
func ContextWithScope(ctx context.Context, s Scope) context.Context {
	return context.WithValue(ctx, scopeKey{}, s)
}

// ScopeFromContext returns any scope associated with the context
// by ContextWithScope.
func ScopeFromContext(ctx context.Context) Scope {
	s, _ := ctx.Value(scopeKey{}).(Scope)
	return s
}
