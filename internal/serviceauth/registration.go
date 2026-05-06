package serviceauth

import (
	"crypto/subtle"
	"errors"
	"strings"
	"time"
)

type RegistrationToken struct {
	Token     string
	ExpiresAt time.Time
}

type RegistrationValidator struct {
	expected string
	now      func() time.Time
}

func NewRegistrationValidator(expected string) *RegistrationValidator {
	return &RegistrationValidator{
		expected: strings.TrimSpace(expected),
		now:      time.Now,
	}
}

func (v *RegistrationValidator) SetNow(now func() time.Time) {
	if now != nil {
		v.now = now
	}
}

func (v *RegistrationValidator) Validate(input RegistrationToken) error {
	if v == nil || strings.TrimSpace(v.expected) == "" {
		return errors.New("bootstrap registration token is not configured")
	}
	if strings.TrimSpace(input.Token) == "" {
		return ErrUnauthenticated
	}
	if input.ExpiresAt.IsZero() || !v.now().UTC().Before(input.ExpiresAt.UTC()) {
		return ErrUnauthenticated
	}
	if subtle.ConstantTimeCompare([]byte(v.expected), []byte(strings.TrimSpace(input.Token))) != 1 {
		return ErrPermissionDenied
	}
	return nil
}
