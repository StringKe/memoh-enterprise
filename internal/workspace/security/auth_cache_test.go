package security

import (
	"testing"
	"time"
)

func TestAuthDecisionCacheExpiresAfterTTL(t *testing.T) {
	now := time.Unix(100, 0)
	cache := NewAuthDecisionCache(func() time.Time { return now })
	key := NewAuthDecisionKey("token", "workspace-1", "run-1", 7)

	if cache.Allowed(key) {
		t.Fatal("cache allowed missing decision")
	}
	cache.Allow(key)
	if !cache.Allowed(key) {
		t.Fatal("cache did not allow fresh decision")
	}
	now = now.Add(5 * time.Second)
	if cache.Allowed(key) {
		t.Fatal("cache allowed expired decision")
	}
}

func TestAuthDecisionKeyHashesToken(t *testing.T) {
	t.Parallel()

	key := NewAuthDecisionKey("token", "workspace-1", "run-1", 7)
	if key.TokenHash == "" || key.TokenHash == "token" {
		t.Fatalf("TokenHash = %q, want hashed token", key.TokenHash)
	}
	if key.WorkspaceID != "workspace-1" || key.RunID != "run-1" || key.LeaseVersion != 7 {
		t.Fatalf("key fields = %+v", key)
	}
}
