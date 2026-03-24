package tenant

import "context"

// Info holds the resolved tenant for the current request.
type Info struct {
	ID   string
	Slug string
	Name string
}

type ctxKey struct{}

// WithInfo stores tenant info in the context.
func WithInfo(ctx context.Context, info Info) context.Context {
	return context.WithValue(ctx, ctxKey{}, info)
}

// FromContext returns the tenant info stored in the context.
func FromContext(ctx context.Context) (Info, bool) {
	info, ok := ctx.Value(ctxKey{}).(Info)
	return info, ok
}

// MustFromContext returns the tenant info or panics.
func MustFromContext(ctx context.Context) Info {
	info, ok := FromContext(ctx)
	if !ok {
		panic("tenant: no tenant info in context")
	}
	return info
}
