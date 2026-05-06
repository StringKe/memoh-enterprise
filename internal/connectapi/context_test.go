package connectapi

import (
	"context"
	"errors"
	"testing"
)

func TestUserIDFromContext(t *testing.T) {
	t.Parallel()

	ctx := WithUserID(context.Background(), " user-1 ")
	got, err := UserIDFromContext(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "user-1" {
		t.Fatalf("want user-1 got %q", got)
	}
}

func TestUserIDFromContextMissing(t *testing.T) {
	t.Parallel()

	_, err := UserIDFromContext(context.Background())
	if !errors.Is(err, ErrUserIDMissing) {
		t.Fatalf("want ErrUserIDMissing got %v", err)
	}
}

func TestSessionIDFromContext(t *testing.T) {
	t.Parallel()

	ctx := WithSessionID(context.Background(), " session-1 ")
	got, err := SessionIDFromContext(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "session-1" {
		t.Fatalf("want session-1 got %q", got)
	}
}

func TestSessionIDFromContextMissing(t *testing.T) {
	t.Parallel()

	_, err := SessionIDFromContext(context.Background())
	if !errors.Is(err, ErrSessionIDMissing) {
		t.Fatalf("want ErrSessionIDMissing got %v", err)
	}
}
