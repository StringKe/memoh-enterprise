package security

import (
	"errors"
	"testing"
	"time"
)

func TestNormalizeExecTimeoutDefaultAndMax(t *testing.T) {
	t.Parallel()

	got, err := NormalizeExecTimeout(0)
	if err != nil {
		t.Fatalf("NormalizeExecTimeout default: %v", err)
	}
	if got != 30*time.Second {
		t.Fatalf("default timeout = %s, want 30s", got)
	}

	got, err = NormalizeExecTimeout(600)
	if err != nil {
		t.Fatalf("NormalizeExecTimeout max: %v", err)
	}
	if got != 600*time.Second {
		t.Fatalf("max timeout = %s, want 600s", got)
	}

	_, err = NormalizeExecTimeout(601)
	if !errors.Is(err, ErrTimeoutTooLarge) {
		t.Fatalf("NormalizeExecTimeout error = %v, want ErrTimeoutTooLarge", err)
	}
}
