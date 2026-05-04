package auth

import "context"

// ctxKey is an unexported type used as a context key. Using a custom type
// avoids the go vet warning for `context.WithValue` with a built-in (e.g.
// string) key, and prevents accidental collisions with keys defined in
// other packages.
type ctxKey int

const claimsKey ctxKey = 0

// WithClaims returns a new context carrying the given JWT claims. The gRPC
// auth interceptor calls this after a successful Validate so downstream
// handlers can read the claims via ClaimsFromContext.
func WithClaims(ctx context.Context, claims *TokenClaims) context.Context {
	return context.WithValue(ctx, claimsKey, claims)
}

// ClaimsFromContext returns the JWT claims attached to ctx, if any.
// ok is false when no claims are present (e.g. when authentication is
// disabled because no auth token was configured at startup).
func ClaimsFromContext(ctx context.Context) (claims *TokenClaims, ok bool) {
	v := ctx.Value(claimsKey)
	if v == nil {
		return nil, false
	}
	c, valid := v.(*TokenClaims)
	if !valid {
		return nil, false
	}
	return c, true
}
