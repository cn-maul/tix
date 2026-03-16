package auth

import "context"

type UserContext struct {
	ID       string
	Username string
	Role     string
}

type contextKey string

const userContextKey contextKey = "auth_user"

func WithUser(ctx context.Context, user *UserContext) context.Context {
	return context.WithValue(ctx, userContextKey, user)
}

func UserFromContext(ctx context.Context) (*UserContext, bool) {
	v := ctx.Value(userContextKey)
	user, ok := v.(*UserContext)
	return user, ok && user != nil
}

func IsAdmin(ctx context.Context) bool {
	user, ok := UserFromContext(ctx)
	return ok && user.Role == "admin"
}
