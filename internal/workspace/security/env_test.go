package security

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestSanitizeEnvAllowsListedKeysAndRequestWins(t *testing.T) {
	t.Parallel()

	env, err := SanitizeEnv(
		[]string{"PATH=/usr/bin", "SECRET_TOKEN=hidden", "TERM=xterm"},
		[]string{"PATH=/opt/bin", "MEMOH_RUN_ID=run-1"},
	)
	if err != nil {
		t.Fatalf("SanitizeEnv: %v", err)
	}
	if !slices.Contains(env, "PATH=/opt/bin") {
		t.Fatalf("PATH override missing from %v", env)
	}
	if !slices.Contains(env, "MEMOH_RUN_ID=run-1") {
		t.Fatalf("MEMOH_RUN_ID missing from %v", env)
	}
	for _, item := range env {
		if strings.HasPrefix(item, "SECRET_TOKEN=") {
			t.Fatalf("secret default leaked into sanitized env: %v", env)
		}
	}
}

func TestSanitizeEnvRejectsForbiddenRequestKey(t *testing.T) {
	t.Parallel()

	_, err := SanitizeEnv(nil, []string{"AWS_SECRET_ACCESS_KEY=secret"})
	if !errors.Is(err, ErrEnvForbidden) {
		t.Fatalf("SanitizeEnv error = %v, want ErrEnvForbidden", err)
	}
}
