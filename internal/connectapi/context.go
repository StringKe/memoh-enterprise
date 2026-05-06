package connectapi

import (
	"context"
	"errors"
	"strings"
)

type (
	userIDContextKey    struct{}
	sessionIDContextKey struct{}
)

var (
	ErrUserIDMissing    = errors.New("user id missing")
	ErrSessionIDMissing = errors.New("session id missing")
)

func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, userIDContextKey{}, strings.TrimSpace(userID))
}

func UserIDFromContext(ctx context.Context) (string, error) {
	userID, _ := ctx.Value(userIDContextKey{}).(string)
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return "", ErrUserIDMissing
	}
	return userID, nil
}

func WithSessionID(ctx context.Context, sessionID string) context.Context {
	return context.WithValue(ctx, sessionIDContextKey{}, strings.TrimSpace(sessionID))
}

func SessionIDFromContext(ctx context.Context) (string, error) {
	sessionID, _ := ctx.Value(sessionIDContextKey{}).(string)
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return "", ErrSessionIDMissing
	}
	return sessionID, nil
}
