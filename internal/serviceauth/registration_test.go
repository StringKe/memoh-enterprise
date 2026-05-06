package serviceauth

import (
	"errors"
	"testing"
	"time"
)

func TestRegistrationTokenValidator(t *testing.T) {
	now := time.Date(2026, 5, 6, 10, 0, 0, 0, time.UTC)
	validator := NewRegistrationValidator("bootstrap")
	validator.SetNow(func() time.Time { return now })

	if err := validator.Validate(RegistrationToken{Token: "", ExpiresAt: now.Add(time.Minute)}); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("missing token error = %v", err)
	}
	if err := validator.Validate(RegistrationToken{Token: "other", ExpiresAt: now.Add(time.Minute)}); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("mismatch token error = %v", err)
	}
	if err := validator.Validate(RegistrationToken{Token: "bootstrap", ExpiresAt: now.Add(-time.Second)}); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("expired token error = %v", err)
	}
	if err := validator.Validate(RegistrationToken{Token: "bootstrap", ExpiresAt: now.Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
}
