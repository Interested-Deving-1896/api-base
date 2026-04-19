// Package requestctx stores and retrieves request-scoped values from the
// Go context.Context.
//
// Anything you want to make available to any function handling a request —
// the request ID, the authenticated user, the trace ID — goes here.
//
// We use a private type for the context key (ctxKey) so that no other
// package can accidentally collide with us by using the same string key.
// If some third-party package also puts a "request_id" string into context,
// it won't collide with ours because their key has a different underlying type.
// This is the standard Go pattern for context values.
//
// Middleware is what populates these values (see internal/shared/middleware).
// Handlers and services retrieve them by calling RequestID(ctx), UserID(ctx),
// etc.
package requestctx

import "context"

type ctxKey string

const (
	requestIDKey ctxKey = "request_id"
	userIDKey    ctxKey = "user_id"
)

func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey, id)
}

func RequestID(ctx context.Context) string {
	if v, ok := ctx.Value(requestIDKey).(string); ok {
		return v
	}
	return ""
}

func WithUserID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, userIDKey, id)
}

func UserID(ctx context.Context) string {
	if v, ok := ctx.Value(userIDKey).(string); ok {
		return v
	}
	return ""
}
